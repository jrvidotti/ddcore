package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// A backup (PRD-01) is one tar archive holding everything a site needs to come
// back except its secrets:
//
//	db.dump            pg_dump -Fc of the whole database
//	files/public/…     every stored object, by storage key
//	files/private/…
//	config/ddcore.json the site's versioned configuration, as it was
//	config/env.json    non-secret DDCORE_* values, and the *names* of secrets
//	manifest.json      versions, migration state, row counts, and a sha256 per entry
//
// The manifest is written last because it describes the others; restore reads
// the whole archive before believing any of it.

const backupFormat = 1

const backupUsage = `Usage: ddcore backup [options]

  --out <file.tar>   where to write (default <DDCORE_BACKUP_DIR>/ddcore-<time>.tar)
  --no-files         database and config only, no stored file bytes
  --maintenance      pause the site for the run, so files and database agree
  --to s3            also upload the archive to the backup bucket (DDCORE_BACKUP_S3_*)
  --keep N           after a successful run, keep only the newest N archives
  --json             print the manifest as JSON

Needs pg_dump on PATH, at least as new as the server. Secrets are never
written: provision DDCORE_SECRET_KEY, DDCORE_SECRET_*, SMTP and S3 credentials
on the restore target separately.`

type backupEntry struct {
	Path   string `json:"path"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type backupApp struct {
	Version string `json:"version,omitempty"`
	Ddcore  string `json:"ddcore,omitempty"`
}

type backupManifest struct {
	Format      int                  `json:"format"`
	DDCore      string               `json:"ddcore"`
	Site        string               `json:"site"`
	Apps        map[string]backupApp `json:"apps"`
	AppOrder    []string             `json:"appOrder"`
	SiteVersion *engine.SiteVersion  `json:"siteVersion,omitempty"`
	// Patches and LastMigration are the migration state the dump carries,
	// repeated here so it can be read without restoring anything.
	Patches       []string         `json:"patches"`
	LastMigration int64            `json:"lastMigration"`
	Postgres      string           `json:"postgres"`
	PgDump        string           `json:"pgDump"`
	Storage       string           `json:"storage"`
	Maintenance   bool             `json:"maintenance"`
	Started       time.Time        `json:"started"`
	Finished      time.Time        `json:"finished"`
	Tables        map[string]int64 `json:"tables"`
	Files         int              `json:"files"`
	FileBytes     int64            `json:"fileBytes"`
	// Secrets are the names of the secrets this site was running with. The
	// restore target needs every one of them; none of their values is here.
	Secrets []string      `json:"secrets"`
	Entries []backupEntry `json:"entries"`
}

type backupEnv struct {
	Values  map[string]string `json:"values"`
	Secrets []string          `json:"secrets"`
}

func cmdBackup(args []string) error {
	fs := newFlagSet("backup")
	out := fs.String("out", "", "archive path")
	noFiles := fs.Bool("no-files", false, "skip stored file bytes")
	maint := fs.Bool("maintenance", false, "pause the site during the backup")
	to := fs.String("to", "", "also upload to: s3")
	keep := fs.Int("keep", -1, "keep only the newest N archives")
	asJSON := fs.Bool("json", false, "print the manifest as JSON")
	fs.Usage = func() { fmt.Println(backupUsage) }
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *to != "" && *to != "s3" {
		return fmt.Errorf("--to: %q is not a destination (s3)", *to)
	}
	e, cfg, err := load(false, false)
	if err != nil {
		return err
	}
	defer e.DB.Close()
	ctx, cancel := signalContext()
	defer cancel()
	if *keep < 0 {
		*keep = cfg.Backup.Keep
	}
	var up storage.Store
	if *to == "s3" {
		if !cfg.Backup.S3Configured {
			return fmt.Errorf("--to s3 needs a bucket: set DDCORE_BACKUP_S3_BUCKET and its credentials, or DDCORE_S3_*")
		}
		if up, err = storage.NewS3(ctx, cfg.Backup.S3); err != nil {
			return err
		}
	}

	started := time.Now()
	logID := startBackupLog(ctx, e, "backup", started)
	man, file, err := runBackup(ctx, e, cfg, backupOpts{Out: *out, NoFiles: *noFiles, Maintenance: *maint})
	if err == nil && up != nil {
		err = uploadBackup(ctx, up, file)
	}
	if err == nil && *keep > 0 {
		pruneBackups(ctx, cfg.Backup.Dir, up, *keep)
	}
	var size int64
	if st, serr := os.Stat(file); serr == nil {
		size = st.Size()
	}
	finishBackupLog(e, logID, file, size, err)
	detail := map[string]any{"archive": filepath.Base(file), "bytes": size, "files": man.Files, "uploaded": up != nil}
	if err != nil {
		detail["error"] = db.RedactError(err)
		e.RecordAudit(ctx, cliActor(), "backup.create", "Denied", "", "", detail)
		return err
	}
	e.RecordAudit(ctx, cliActor(), "backup.create", "Allowed", "", "", detail)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(man)
	}
	fmt.Printf("backup written: %s (%s, %d tables, %d files) in %s\n", file, humanBytes(size), len(man.Tables), man.Files, time.Since(started).Round(time.Millisecond))
	if up != nil {
		fmt.Printf("uploaded to s3://%s/%s\n", cfg.Backup.S3.Bucket, path.Join(cfg.Backup.S3.Prefix, filepath.Base(file)))
	}
	if len(man.Secrets) > 0 {
		fmt.Printf("not in the archive — provision on the target: %s\n", strings.Join(man.Secrets, ", "))
	}
	return nil
}

type backupOpts struct {
	Out         string
	NoFiles     bool
	Maintenance bool
}

// runBackup writes the archive and returns its manifest and path. A failure
// leaves no archive behind: the file is written under a temporary name and
// renamed only once the manifest is in it.
func runBackup(ctx context.Context, e *engine.Engine, cfg *config.File, o backupOpts) (*backupManifest, string, error) {
	man := &backupManifest{Format: backupFormat, DDCore: engine.Version, Site: e.SiteTitle(), Apps: map[string]backupApp{},
		Storage: e.Storage().Backend(), Started: time.Now().UTC(), Tables: map[string]int64{}, Patches: []string{}}
	out := o.Out
	if out == "" {
		out = filepath.Join(cfg.Backup.Dir, "ddcore-"+man.Started.Format("20060102-150405")+".tar")
	}
	for _, n := range e.AppOrder() {
		man.AppOrder = append(man.AppOrder, n)
		if am := e.Snap.Apps[n]; am != nil {
			man.Apps[n] = backupApp{Version: am.Version, Ddcore: am.Ddcore}
		}
	}

	dump, err := findPGTool("pg_dump")
	if err != nil {
		return man, out, err
	}
	clientMajor, clientVersion, err := dump.Version(ctx)
	if err != nil {
		return man, out, err
	}
	man.PgDump = clientVersion
	var serverNum int
	if err := e.DB.Pool.QueryRow(ctx, `SELECT current_setting('server_version'), current_setting('server_version_num')::int`).Scan(&man.Postgres, &serverNum); err != nil {
		return man, out, err
	}
	if clientMajor < serverNum/10000 {
		return man, out, fmt.Errorf("pg_dump %d cannot dump a PostgreSQL %d server: install postgresql-client-%d or newer", clientMajor, serverNum/10000, serverNum/10000)
	}

	if o.Maintenance {
		prev := e.Maintenance(ctx)
		if !prev.Enabled {
			if _, err := e.SetMaintenance(ctx, true, "Backup in progress", cliActor()); err != nil {
				return man, out, err
			}
			defer func() {
				if _, err := e.SetMaintenance(context.Background(), false, "", cliActor()); err != nil {
					fmt.Fprintln(os.Stderr, "warning: could not leave maintenance mode:", db.RedactError(err))
				}
			}()
			// every server rereads the flag within its cache window; give them
			// that, and give a request already inside a write the same grace
			fmt.Println("maintenance on; waiting for servers to pause…")
			sleepCtx(ctx, 3*time.Second)
		}
		man.Maintenance = true
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o750); err != nil {
		return man, out, err
	}
	partial := out + ".partial"
	f, err := os.OpenFile(partial, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return man, out, err
	}
	ok := false
	defer func() {
		f.Close()
		if !ok {
			os.Remove(partial)
		}
	}()
	tw := tar.NewWriter(f)

	// One snapshot for everything read from the database: the dump, the row
	// counts and the migration state all describe the same instant, which is
	// what lets a restore compare its counts with these.
	tx, err := e.DB.Pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return man, out, err
	}
	defer tx.Rollback(context.Background())
	var snapshot string
	if err := tx.QueryRow(ctx, `SELECT pg_export_snapshot()`).Scan(&snapshot); err != nil {
		return man, out, err
	}
	if err := backupState(ctx, tx, man); err != nil {
		return man, out, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(out), ".db-*.dump")
	if err != nil {
		return man, out, err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if err := dump.Run(ctx, cfg.DSN, []string{"--format=custom", "--no-owner", "--no-acl", "--snapshot=" + snapshot}, tmp); err != nil {
		return man, out, err
	}
	tx.Rollback(ctx)
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return man, out, err
	}
	st, err := tmp.Stat()
	if err != nil {
		return man, out, err
	}
	if err := addEntry(tw, man, "db.dump", st.Size(), tmp); err != nil {
		return man, out, err
	}

	if !o.NoFiles {
		store := e.Storage()
		for _, prefix := range []string{"public/", "private/"} {
			err := store.List(ctx, prefix, func(key string, info storage.Info) error {
				rc, oinfo, err := store.Open(ctx, key)
				if err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
				defer rc.Close()
				if err := addEntry(tw, man, "files/"+key, oinfo.Size, rc); err != nil {
					return fmt.Errorf("%s: %w", key, err)
				}
				man.Files++
				man.FileBytes += oinfo.Size
				return nil
			})
			if err != nil {
				return man, out, err
			}
		}
	}

	if cfgPath, err := configPath(); err == nil {
		if b, err := os.ReadFile(cfgPath); err == nil {
			if err := addBytes(tw, man, "config/ddcore.json", b); err != nil {
				return man, out, err
			}
		}
	}
	envSnap := captureEnv()
	man.Secrets = envSnap.Secrets
	envJSON, _ := json.MarshalIndent(envSnap, "", "  ")
	if err := addBytes(tw, man, "config/env.json", envJSON); err != nil {
		return man, out, err
	}

	man.Finished = time.Now().UTC()
	manJSON, err := json.MarshalIndent(man, "", "  ")
	if err != nil {
		return man, out, err
	}
	// the manifest's own checksum cannot be inside it; restore checks every
	// other entry against it and refuses anything it does not list
	if err := writeTarFile(tw, "manifest.json", int64(len(manJSON)), strings.NewReader(string(manJSON)), nil); err != nil {
		return man, out, err
	}
	if err := tw.Close(); err != nil {
		return man, out, err
	}
	if err := f.Sync(); err != nil {
		return man, out, err
	}
	if err := f.Close(); err != nil {
		return man, out, err
	}
	if err := os.Rename(partial, out); err != nil {
		return man, out, err
	}
	ok = true
	return man, out, nil
}

// backupState reads what the manifest says about the database, inside the
// dump's snapshot.
func backupState(ctx context.Context, q db.Querier, man *backupManifest) error {
	sv, err := engine.LastSiteVersion(ctx, q)
	if err != nil {
		return err
	}
	man.SiteVersion = sv
	if err := q.QueryRow(ctx, `SELECT coalesce(max(id), 0) FROM ddcore_migration`).Scan(&man.LastMigration); err != nil && !db.UndefinedTable(err) {
		return err
	}
	rows, err := db.Select(ctx, q, `SELECT app, name FROM ddcore_patch ORDER BY app, name`)
	if err != nil && !db.UndefinedTable(err) {
		return err
	}
	for _, r := range rows {
		man.Patches = append(man.Patches, db.Str(r["app"])+"/"+db.Str(r["name"]))
	}
	tables, err := db.Select(ctx, q, `SELECT table_name FROM information_schema.tables
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE' AND table_name LIKE 'tab\_%' ORDER BY 1`)
	if err != nil {
		return err
	}
	for _, t := range tables {
		name := db.Str(t["table_name"])
		var n int64
		if err := q.QueryRow(ctx, `SELECT count(*) FROM `+pgx.Identifier{name}.Sanitize()).Scan(&n); err != nil {
			return err
		}
		man.Tables[name] = n
	}
	return nil
}

// hashingReader checksums what passes through it.
type hashingReader struct {
	r io.Reader
	h hash.Hash
	n int64
}

func (h *hashingReader) Read(p []byte) (int, error) {
	n, err := h.r.Read(p)
	h.h.Write(p[:n])
	h.n += int64(n)
	return n, err
}

func addEntry(tw *tar.Writer, man *backupManifest, name string, size int64, r io.Reader) error {
	hr := &hashingReader{r: r, h: sha256.New()}
	if err := writeTarFile(tw, name, size, hr, nil); err != nil {
		return err
	}
	if hr.n != size {
		// an object that grew or shrank while it was read: the header already
		// promised a size, so the archive would be corrupt from here on
		return fmt.Errorf("%s changed while it was read (%d bytes, expected %d); retry with --maintenance", name, hr.n, size)
	}
	man.Entries = append(man.Entries, backupEntry{Path: name, Bytes: size, SHA256: hex.EncodeToString(hr.h.Sum(nil))})
	return nil
}

func addBytes(tw *tar.Writer, man *backupManifest, name string, b []byte) error {
	return addEntry(tw, man, name, int64(len(b)), strings.NewReader(string(b)))
}

func writeTarFile(tw *tar.Writer, name string, size int64, r io.Reader, mod *time.Time) error {
	t := time.Now()
	if mod != nil {
		t = *mod
	}
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size, ModTime: t, Typeflag: tar.TypeReg, Format: tar.FormatPAX}); err != nil {
		return err
	}
	_, err := io.CopyN(tw, r, size)
	if errors.Is(err, io.EOF) {
		return fmt.Errorf("%s: shorter than its %d bytes", name, size)
	}
	return err
}

func configPath() (string, error) {
	_, p, err := config.Load(".")
	return p, err
}

// secretEnv is what never leaves the process: anything that authenticates.
// The connection string is on the list because it carries the password.
var secretEnv = regexp.MustCompile(`(?i)(PASSWORD|SECRET|ACCESS_KEY|TOKEN|_KEY$|^DDCORE_DSN$|^DATABASE_URL$)`)

// captureEnv records the environment a site ran with, minus its secrets,
// whose names are kept so the restore target knows what to provision.
func captureEnv() backupEnv {
	out := backupEnv{Values: map[string]string{}, Secrets: []string{}}
	for _, kv := range os.Environ() {
		k, v, _ := strings.Cut(kv, "=")
		if v == "" || !(strings.HasPrefix(k, "DDCORE_") || k == "DATABASE_URL" || k == "PORT") {
			continue
		}
		if secretEnv.MatchString(k) {
			out.Secrets = append(out.Secrets, k)
			continue
		}
		out.Values[k] = v
	}
	sort.Strings(out.Secrets)
	return out
}

func uploadBackup(ctx context.Context, store storage.Store, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := store.Put(ctx, filepath.Base(file), f, st.Size(), "application/x-tar"); err != nil {
		return fmt.Errorf("upload: %w", err)
	}
	return nil
}

var archiveName = regexp.MustCompile(`^ddcore-\d{8}-\d{6}\.tar$`)

// pruneBackups keeps the newest `keep` archives in the local directory and in
// the bucket, judged by name — the timestamp in it — and only among files
// whose names this command generates, so nothing else in either place is
// touched. Failing to prune is reported, not fatal: the backup itself worked.
func pruneBackups(ctx context.Context, dir string, up storage.Store, keep int) {
	if entries, err := os.ReadDir(dir); err == nil {
		var names []string
		for _, d := range entries {
			if !d.IsDir() && archiveName.MatchString(d.Name()) {
				names = append(names, d.Name())
			}
		}
		for _, n := range oldest(names, keep) {
			if err := os.Remove(filepath.Join(dir, n)); err != nil {
				fmt.Fprintln(os.Stderr, "warning: prune:", err)
			} else {
				fmt.Println("pruned", filepath.Join(dir, n))
			}
		}
	}
	if up == nil {
		return
	}
	var names []string
	if err := up.List(ctx, "", func(key string, _ storage.Info) error {
		if archiveName.MatchString(key) {
			names = append(names, key)
		}
		return nil
	}); err != nil {
		fmt.Fprintln(os.Stderr, "warning: prune: listing the bucket:", err)
		return
	}
	for _, n := range oldest(names, keep) {
		if err := up.Delete(ctx, n); err != nil {
			fmt.Fprintln(os.Stderr, "warning: prune:", err)
		} else {
			fmt.Println("pruned s3 object", n)
		}
	}
}

func oldest(names []string, keep int) []string {
	if len(names) <= keep {
		return nil
	}
	sort.Strings(names)
	return names[:len(names)-keep]
}

// startBackupLog and finishBackupLog keep ddcore_backup_log, which is what
// doctor reads to say how old the newest backup is. Failing to write it never
// fails the backup.
func startBackupLog(ctx context.Context, e *engine.Engine, kind string, started time.Time) int64 {
	if err := db.EnsureOps(ctx, e.DB.Pool); err != nil {
		return 0
	}
	var id int64
	if err := e.DB.Pool.QueryRow(ctx, `INSERT INTO ddcore_backup_log (kind, started) VALUES ($1, $2) RETURNING id`, kind, started).Scan(&id); err != nil {
		return 0
	}
	return id
}

func finishBackupLog(e *engine.Engine, id int64, location string, size int64, runErr error) {
	if id == 0 {
		return
	}
	msg := ""
	if runErr != nil {
		msg = db.RedactError(runErr)
	}
	// a fresh context: the run's own may be the thing that was cancelled
	e.DB.Pool.Exec(context.Background(), `UPDATE ddcore_backup_log SET finished = now(), location = $2, bytes = $3, ok = $4, error = $5 WHERE id = $1`,
		id, location, size, runErr == nil, msg)
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}

func sleepCtx(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}
