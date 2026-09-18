package main

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/storage"
)

const restoreUsage = `Usage: ddcore restore <archive.tar | s3:<name>> [options]

Restores into the database and storage this directory is configured for
(DDCORE_DSN, DDCORE_STORAGE, dataDir). The default target is an isolated,
empty database: pointing a copy of .env at a new database and a new data
directory is how a restore drill runs beside production.

  --verify-only       check every checksum and print the manifest; change nothing
  --force             restore over a database that already holds a site
  --no-files          skip stored file bytes
  --no-migrate        do not run migrate after the restore
  --keep-sessions     keep the archived sessions (by default everyone signs in again)
  --online            leave maintenance off afterwards (by default the site stays paused)
  --smoke             after restoring, check readiness, row counts, files and a login
  --smoke-user <u>    sign in as this user in the smoke check; the password is
                      read from DDCORE_SMOKE_PASSWORD, never from the command line
  --json              machine-readable result

The restored site stays in maintenance mode until ` + "`ddcore maintenance off`" + `, so
nothing — no worker, no scheduled job, no webhook — acts on restored data before
someone has looked at it.`

type restorePhase struct {
	Name     string  `json:"name"`
	Seconds  float64 `json:"seconds"`
	Detail   string  `json:"detail,omitempty"`
	started  time.Time
	finished bool
}

type smokeCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

type restoreResult struct {
	Archive     string          `json:"archive"`
	Manifest    *backupManifest `json:"manifest"`
	Phases      []*restorePhase `json:"phases"`
	Seconds     float64         `json:"seconds"`
	Maintenance bool            `json:"maintenance"`
	Smoke       []smokeCheck    `json:"smoke,omitempty"`
	SmokeOK     *bool           `json:"smokeOk,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
}

func (r *restoreResult) phase(name string) *restorePhase {
	p := &restorePhase{Name: name, started: time.Now()}
	r.Phases = append(r.Phases, p)
	return p
}

func (p *restorePhase) done(detail string) {
	p.Seconds = time.Since(p.started).Seconds()
	p.Detail = detail
	p.finished = true
}

func signalContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func cmdRestore(args []string) error {
	fs := newFlagSet("restore")
	verifyOnly := fs.Bool("verify-only", false, "only verify the archive")
	force := fs.Bool("force", false, "restore over an existing site")
	noFiles := fs.Bool("no-files", false, "skip stored file bytes")
	noMigrate := fs.Bool("no-migrate", false, "do not migrate after restoring")
	keepSessions := fs.Bool("keep-sessions", false, "keep archived sessions")
	online := fs.Bool("online", false, "leave maintenance off afterwards")
	smoke := fs.Bool("smoke", false, "run the smoke check afterwards")
	smokeUser := fs.String("smoke-user", "", "user to sign in as in the smoke check")
	asJSON := fs.Bool("json", false, "JSON output")
	fs.Usage = func() { fmt.Println(restoreUsage) }
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("%s", restoreUsage)
	}
	cfg, _, err := config.Load(".")
	if err != nil {
		return err
	}
	ctx, cancel := signalContext()
	defer cancel()

	res := &restoreResult{Archive: fs.Arg(0)}
	begin := time.Now()
	dbReplaced := false
	report := func(runErr error) error {
		res.Seconds = time.Since(begin).Seconds()
		if w := afterDatabaseWarning(dbReplaced, runErr); w != "" {
			res.Warnings = append(res.Warnings, w)
		}
		if *asJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(res)
		} else {
			printRestore(res)
		}
		if runErr != nil {
			return runErr
		}
		if res.SmokeOK != nil && !*res.SmokeOK {
			return fmt.Errorf("smoke check failed")
		}
		return nil
	}

	// 1. fetch and verify, before anything on the target is touched
	work, err := os.MkdirTemp(filepath.Dir(cfg.Backup.Dir), ".restore-*")
	if err != nil {
		if err := os.MkdirAll(filepath.Dir(cfg.Backup.Dir), 0o750); err != nil {
			return err
		}
		if work, err = os.MkdirTemp(filepath.Dir(cfg.Backup.Dir), ".restore-*"); err != nil {
			return err
		}
	}
	defer os.RemoveAll(work)
	p := res.phase("fetch")
	archive, err := fetchArchive(ctx, cfg, fs.Arg(0), work)
	if err != nil {
		return err
	}
	p.done(archive)
	p = res.phase("verify")
	man, err := extractArchive(archive, filepath.Join(work, "x"))
	if err != nil {
		return fmt.Errorf("archive refused: %w", err)
	}
	res.Manifest = man
	p.done(fmt.Sprintf("%d entries", len(man.Entries)))
	if *verifyOnly {
		return report(nil)
	}
	dir := filepath.Join(work, "x")

	if cfg.DSN == "" {
		return fmt.Errorf("dsn not configured: set DDCORE_DSN to the database to restore into")
	}
	// The same comparison the engine makes after the restore, made before it:
	// finding out that this binary refuses the database once the target's own
	// data is already gone would be the worst time to find out.
	if !allowOlderBinary() {
		archived := man.SiteVersion
		if archived == nil {
			archived = &engine.SiteVersion{Core: man.DDCore}
		}
		apps, err := loadedAppVersions(ctx, cfg)
		if err != nil {
			return err
		}
		if older := engine.OlderThanSite(archived, engine.Version, apps); len(older) > 0 {
			return fmt.Errorf("this binary is older than the archived site: %s. Restore with that release or newer, or pass --allow-older-binary", strings.Join(older, "; "))
		}
	}
	restoreTool, err := findPGTool("pg_restore")
	if err != nil {
		return err
	}

	// 2. the target
	target, err := db.Open(ctx, cfg.DSN)
	if err != nil {
		return fmt.Errorf("target database: %s", db.RedactError(err))
	}
	var occupied int
	if err := target.Pool.QueryRow(ctx, `SELECT count(*) FROM information_schema.tables
		WHERE table_schema = current_schema() AND (table_name LIKE 'tab\_%' OR table_name LIKE 'ddcore\_%')`).Scan(&occupied); err != nil {
		target.Close()
		return err
	}
	if occupied > 0 && !*force {
		target.Close()
		return fmt.Errorf("the target database %s already holds a site (%d tables): restore into an empty database, or pass --force to replace it", db.RedactDSN(cfg.DSN), occupied)
	}
	pausedHere := false
	if occupied > 0 {
		// pause whatever is serving the target before its tables are dropped
		// under it; the servers see this within their cache window. A target
		// the operator had already paused stays paused whatever happens next.
		var wasPaused bool
		if err := target.Pool.QueryRow(ctx, `SELECT enabled FROM ddcore_maintenance WHERE id = 1`).Scan(&wasPaused); err != nil && !errors.Is(err, pgx.ErrNoRows) && !db.UndefinedTable(err) {
			wasPaused = true // unknown: never switch off a pause this restore did not make
		}
		if err := setMaintenanceSQL(ctx, target, true, "Restore in progress"); err != nil {
			res.Warnings = append(res.Warnings, "could not pause the target: "+db.RedactError(err))
		} else {
			pausedHere = !wasPaused
			sleepCtx(ctx, 3*time.Second)
		}
	}

	// 3. database — one transaction, so a failed restore leaves the target as it was
	p = res.phase("database")
	err = restoreTool.Run(ctx, cfg.DSN, []string{"--clean", "--if-exists", "--no-owner", "--no-acl", "--exit-on-error", "--single-transaction",
		filepath.Join(dir, "db.dump")}, nil)
	if err != nil {
		// the transaction rolled back, so the old site is intact: let it serve
		// again rather than leave it refusing every write
		if pausedHere {
			if uerr := setMaintenanceSQL(context.WithoutCancel(ctx), target, false, ""); uerr != nil {
				res.Warnings = append(res.Warnings, "the target is still paused; run `ddcore maintenance off`: "+db.RedactError(uerr))
			}
		}
		target.Close()
		return report(err)
	}
	p.done(fmt.Sprintf("%d tables", len(man.Tables)))
	dbReplaced = true

	// 4. files
	if !*noFiles && man.Files > 0 {
		p = res.phase("files")
		store, err := storage.New(ctx, cfg.Storage, cfg.DataDir)
		if err != nil {
			target.Close()
			return report(err)
		}
		n, err := restoreFiles(ctx, store, dir, man)
		if err != nil {
			target.Close()
			return report(err)
		}
		p.done(fmt.Sprintf("%d files to %s", n, store.Backend()))
	}

	// 5. state: nobody is signed in to a restored site, and it stays paused
	if err := setMaintenanceSQL(ctx, target, !*online, "Restored from backup"); err != nil {
		target.Close()
		return report(err)
	}
	res.Maintenance = !*online
	if !*keepSessions {
		if _, err := target.Pool.Exec(ctx, `DELETE FROM ddcore_session`); err != nil && !db.UndefinedTable(err) {
			res.Warnings = append(res.Warnings, "sessions were not cleared: "+db.RedactError(err))
		}
	}
	target.Close()

	// 6. the engine: the version ledger check runs here, then migrate
	p = res.phase("migrate")
	e, _, err := load(false, false)
	if err != nil {
		return report(err)
	}
	defer e.DB.Close()
	if *noMigrate {
		p.done("skipped")
	} else {
		mr, err := e.Migrate(ctx, false)
		if err != nil {
			return report(fmt.Errorf("migrate after restore: %w", err))
		}
		p.done(fmt.Sprintf("%d DDL, %d patches", len(mr.DDL), len(mr.Patches)))
		showAdminPassword(mr)
	}
	e.RecordAudit(ctx, cliActor(), "backup.restore", "Allowed", "", "", map[string]any{
		"archive": filepath.Base(res.Archive), "ddcore": man.DDCore, "started": man.Started, "files": man.Files})
	logID := startBackupLog(ctx, e, "restore", begin)
	finishBackupLog(e, logID, res.Archive, 0, nil)

	if *smoke {
		p = res.phase("smoke")
		res.Smoke = smokeCheckSite(ctx, e, man, *smokeUser)
		ok := true
		for _, c := range res.Smoke {
			ok = ok && c.OK
		}
		res.SmokeOK = &ok
		p.done("")
	}
	return report(nil)
}

// loadedAppVersions compiles the configured apps without a database, for the
// versions they declare.
func loadedAppVersions(ctx context.Context, cfg *config.File) (map[string]string, error) {
	var apps []js.App
	for _, dir := range cfg.Apps {
		apps = append(apps, js.App{Name: js.AppName(dir), Dir: dir})
	}
	e, err := engine.New(ctx, engine.Config{Apps: apps, DataDir: cfg.DataDir, LogOut: io.Discard, Workers: 0})
	if err != nil {
		return nil, fmt.Errorf("loading the apps: %w", err)
	}
	out := map[string]string{}
	for name, am := range e.Snap.Apps {
		if am != nil && am.Version != "" {
			out[name] = am.Version
		}
	}
	return out, nil
}

// fetchArchive returns a local path for the source: the file itself, or a
// download of `s3:<name>` from the backup bucket.
func fetchArchive(ctx context.Context, cfg *config.File, src, work string) (string, error) {
	name, ok := strings.CutPrefix(src, "s3:")
	if !ok {
		if _, err := os.Stat(src); err != nil {
			return "", err
		}
		return src, nil
	}
	if !cfg.Backup.S3Configured {
		return "", fmt.Errorf("s3: no backup bucket configured (DDCORE_BACKUP_S3_* or DDCORE_S3_*)")
	}
	store, err := storage.NewS3(ctx, cfg.Backup.S3)
	if err != nil {
		return "", err
	}
	rc, _, err := store.Open(ctx, strings.TrimPrefix(name, "/"))
	if err != nil {
		return "", fmt.Errorf("s3:%s: %w", name, err)
	}
	defer rc.Close()
	local := filepath.Join(work, path.Base(name))
	f, err := os.Create(local)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := io.Copy(f, rc); err != nil {
		return "", err
	}
	return local, f.Close()
}

// extractArchive unpacks into dir and verifies every entry against the
// manifest. It refuses an entry the manifest does not list, a listed entry
// that is missing, a checksum or size that differs, and any path that could
// land outside dir.
func extractArchive(archive, dir string) (*backupManifest, error) {
	f, err := os.Open(archive)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	type seen struct {
		bytes int64
		sum   string
	}
	got := map[string]seen{}
	var manJSON []byte
	tr := tar.NewReader(f)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("not a readable backup archive: %w", err)
		}
		if h.Typeflag != tar.TypeReg {
			return nil, fmt.Errorf("%s: only regular files belong in a backup", h.Name)
		}
		name := h.Name
		if name == "manifest.json" {
			if manJSON, err = io.ReadAll(io.LimitReader(tr, 64<<20)); err != nil {
				return nil, err
			}
			continue
		}
		if !safeEntry(name) {
			return nil, fmt.Errorf("%q: unexpected path in archive", name)
		}
		if _, dup := got[name]; dup {
			return nil, fmt.Errorf("%s appears twice", name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
			return nil, err
		}
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return nil, err
		}
		hsh := sha256.New()
		n, err := io.Copy(io.MultiWriter(out, hsh), tr)
		out.Close()
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		got[name] = seen{n, hex.EncodeToString(hsh.Sum(nil))}
	}
	if manJSON == nil {
		return nil, fmt.Errorf("no manifest.json: not a ddcore backup, or a truncated one")
	}
	var man backupManifest
	if err := json.Unmarshal(manJSON, &man); err != nil {
		return nil, fmt.Errorf("manifest.json: %w", err)
	}
	if man.Format != backupFormat {
		return nil, fmt.Errorf("archive format %d is not supported by this binary (format %d)", man.Format, backupFormat)
	}
	listed := map[string]bool{}
	for _, en := range man.Entries {
		listed[en.Path] = true
		g, ok := got[en.Path]
		if !ok {
			return nil, fmt.Errorf("%s is listed in the manifest but missing", en.Path)
		}
		if g.bytes != en.Bytes || g.sum != en.SHA256 {
			return nil, fmt.Errorf("%s does not match its checksum: the archive is damaged or was altered", en.Path)
		}
	}
	for name := range got {
		if !listed[name] {
			return nil, fmt.Errorf("%s is not listed in the manifest", name)
		}
	}
	if !listed["db.dump"] {
		return nil, fmt.Errorf("the archive holds no database dump")
	}
	return &man, nil
}

// safeEntry admits exactly the layout backup writes.
func safeEntry(name string) bool {
	if name != path.Clean(name) || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	switch name {
	case "db.dump", "config/ddcore.json", "config/env.json":
		return true
	}
	if key, ok := strings.CutPrefix(name, "files/"); ok {
		sub, file, ok := strings.Cut(key, "/")
		return ok && (sub == "public" || sub == "private") && file != "" && !strings.HasPrefix(file, ".")
	}
	return false
}

func restoreFiles(ctx context.Context, store storage.Store, dir string, man *backupManifest) (int, error) {
	n := 0
	for _, en := range man.Entries {
		key, ok := strings.CutPrefix(en.Path, "files/")
		if !ok {
			continue
		}
		f, err := os.Open(filepath.Join(dir, filepath.FromSlash(en.Path)))
		if err != nil {
			return n, err
		}
		err = store.Put(ctx, key, f, en.Bytes, "")
		f.Close()
		if err != nil {
			return n, fmt.Errorf("%s: %w", key, err)
		}
		n++
	}
	return n, nil
}

// setMaintenanceSQL writes the flag without an engine: during a restore there
// is no schema an engine could load yet, or the one there is about to go.
func setMaintenanceSQL(ctx context.Context, d *db.DB, on bool, reason string) error {
	if err := db.EnsureOps(ctx, d.Pool); err != nil {
		return err
	}
	if !on {
		reason = ""
	}
	_, err := d.Pool.Exec(ctx, `INSERT INTO ddcore_maintenance (id, enabled, reason, since, actor)
		VALUES (1, $1, $2, CASE WHEN $1 THEN now() END, $3)
		ON CONFLICT (id) DO UPDATE SET enabled = EXCLUDED.enabled, reason = EXCLUDED.reason, since = EXCLUDED.since, actor = EXCLUDED.actor`,
		on, reason, cliActor())
	return err
}

// smokeCheckSite is the minimum a restored site has to pass before anyone
// relies on it: the database answers, nothing archived went missing, stored
// files open, and someone can sign in. An app's own critical flow is the
// drill's next step, and belongs to the app (see docs/agent/backup.md).
func smokeCheckSite(ctx context.Context, e *engine.Engine, man *backupManifest, user string) []smokeCheck {
	var out []smokeCheck
	h := e.Ready(ctx)
	out = append(out, smokeCheck{Name: "database ready", OK: h.OK, Detail: h.Error})

	var short []string
	names := make([]string, 0, len(man.Tables))
	for t := range man.Tables {
		names = append(names, t)
	}
	sort.Strings(names)
	for _, t := range names {
		var n int64
		// identifiers from our own manifest, but quoted all the same
		if err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM "`+strings.ReplaceAll(t, `"`, `""`)+`"`).Scan(&n); err != nil {
			short = append(short, fmt.Sprintf("%s: %s", t, db.RedactError(err)))
			continue
		}
		// fewer is loss; more is what a migration's fixtures or this restore's
		// own audit row may add
		if n < man.Tables[t] {
			short = append(short, fmt.Sprintf("%s: %d rows, archived %d", t, n, man.Tables[t]))
		}
	}
	out = append(out, smokeCheck{Name: "row counts", OK: len(short) == 0, Detail: strings.Join(short, "; ")})

	rows, err := db.Select(ctx, e.DB.Pool, `SELECT file_url FROM tab_file WHERE coalesce(file_url, '') <> '' ORDER BY random() LIMIT 20`)
	if err != nil {
		out = append(out, smokeCheck{Name: "files open", OK: false, Detail: db.RedactError(err)})
	} else {
		var missing []string
		for _, r := range rows {
			u := db.Str(r["file_url"])
			key, ok := storage.KeyFromURL(u)
			if !ok {
				continue
			}
			rc, _, err := e.Storage().Open(ctx, key)
			if err != nil {
				missing = append(missing, u)
				continue
			}
			rc.Close()
		}
		out = append(out, smokeCheck{Name: "files open", OK: len(missing) == 0,
			Detail: fmt.Sprintf("%d sampled", len(rows)) + map[bool]string{true: "; missing: " + strings.Join(missing, ", "), false: ""}[len(missing) > 0]})
	}

	if user != "" {
		pw := os.Getenv("DDCORE_SMOKE_PASSWORD")
		if pw == "" {
			out = append(out, smokeCheck{Name: "login", OK: false, Detail: "set DDCORE_SMOKE_PASSWORD"})
		} else if sid, err := e.Login(ctx, user, pw, engine.LoginFrom{IP: "restore-smoke", UserAgent: "ddcore restore --smoke"}); err != nil {
			out = append(out, smokeCheck{Name: "login", OK: false, Detail: err.Error()})
		} else {
			// the check must not leave a usable session behind
			e.DB.Pool.Exec(ctx, `DELETE FROM ddcore_session WHERE sid = $1`, sid)
			out = append(out, smokeCheck{Name: "login", OK: true, Detail: user})
		}
	} else {
		var n int
		err := e.DB.Pool.QueryRow(ctx, `SELECT count(*) FROM tab_user u WHERE u.enabled AND (u.name = 'Admin'
			OR EXISTS (SELECT 1 FROM tab_has_role r WHERE r.parent = u.name AND r.role = 'System Manager'))`).Scan(&n)
		detail := fmt.Sprintf("%d enabled admin(s); pass --smoke-user to sign in for real", n)
		if err != nil {
			detail = db.RedactError(err)
		}
		out = append(out, smokeCheck{Name: "admin exists", OK: err == nil && n > 0, Detail: detail})
	}
	return out
}

func printRestore(r *restoreResult) {
	if m := r.Manifest; m != nil {
		fmt.Printf("archive: %s\n  made %s by ddcore %s from %q (postgres %s, %s storage)\n",
			r.Archive, m.Started.Format(time.RFC3339), m.DDCore, m.Site, m.Postgres, m.Storage)
		apps := make([]string, 0, len(m.AppOrder))
		for _, n := range m.AppOrder {
			apps = append(apps, n+" "+m.Apps[n].Version)
		}
		fmt.Printf("  apps: %s\n  %d tables, %d files (%s)\n", strings.Join(apps, ", "), len(m.Tables), m.Files, humanBytes(m.FileBytes))
		if len(m.Secrets) > 0 {
			fmt.Printf("  provision on this target: %s\n", strings.Join(m.Secrets, ", "))
		}
	}
	for _, p := range r.Phases {
		if !p.finished {
			fmt.Printf("  %-9s failed after %.1fs\n", p.Name, time.Since(p.started).Seconds())
			continue
		}
		fmt.Printf("  %-9s %6.1fs  %s\n", p.Name, p.Seconds, p.Detail)
	}
	for _, c := range r.Smoke {
		mark := "ok  "
		if !c.OK {
			mark = "FAIL"
		}
		fmt.Printf("  [%s] %s %s\n", mark, c.Name, c.Detail)
	}
	for _, w := range r.Warnings {
		fmt.Println("  warning:", w)
	}
	fmt.Printf("total: %.1fs\n", r.Seconds)
	if r.Maintenance {
		fmt.Println("the restored site is in maintenance mode: check it, then `ddcore maintenance off`")
	}
}

// afterDatabaseWarning is what the operator is told when a restore stops once
// the database has been replaced. Steps 4 and later are not undone — the old
// site is gone and the new one is behind the pause — so the failure alone,
// which reads like "nothing happened", is not the whole story.
func afterDatabaseWarning(dbReplaced bool, runErr error) string {
	if !dbReplaced || runErr == nil {
		return ""
	}
	return "the database was restored before this failure and the target is still paused: finish or redo the restore, then `ddcore maintenance off`"
}
