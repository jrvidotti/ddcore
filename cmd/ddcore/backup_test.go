package main

import (
	"archive/tar"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"errors"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/storage"
)

func TestSplitDSNPassword(t *testing.T) {
	for _, tc := range []struct{ in, conn, pw string }{
		{"postgres://u:p%40ss@h:5432/d?sslmode=disable", "postgres://u@h:5432/d?sslmode=disable", "p@ss"},
		{"postgres://u@h/d", "postgres://u@h/d", ""},
		{"host=h user=u password='a b''c' dbname=d", "host=h user=u  dbname=d", "a b'c"},
		{"host=h password=plain dbname=d", "host=h  dbname=d", "plain"},
	} {
		conn, pw := splitDSNPassword(tc.in)
		if conn != tc.conn || pw != tc.pw {
			t.Errorf("splitDSNPassword(%q) = %q, %q; want %q, %q", tc.in, conn, pw, tc.conn, tc.pw)
		}
	}
}

// An archive is untrusted input: only the layout backup writes is accepted.
func TestSafeEntry(t *testing.T) {
	for name, want := range map[string]bool{
		"db.dump": true, "config/env.json": true, "files/public/a.png": true, "files/private/x.pdf": true,
		"../db.dump": false, "/etc/passwd": false, "files/public/../../x": false, "files/other/a": false,
		"files/public/.env": false, "files/public/": false, "config/other.json": false, "files/public/a/b": true,
	} {
		if got := safeEntry(name); got != want {
			t.Errorf("safeEntry(%q) = %v, want %v", name, got, want)
		}
	}
}

// Secrets leave the archive by name only.
func TestCaptureEnvKeepsSecretsOut(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "master")
	t.Setenv("DDCORE_SECRET_STRIPE_KEY", "sk_live")
	t.Setenv("DDCORE_S3_SECRET_KEY", "s3secret")
	t.Setenv("DDCORE_SMTP_PASSWORD", "smtp")
	t.Setenv("DDCORE_DSN", "postgres://u:pw@h/d")
	t.Setenv("DDCORE_URL", "https://example.com")
	env := captureEnv()
	for _, k := range []string{"DDCORE_SECRET_KEY", "DDCORE_SECRET_STRIPE_KEY", "DDCORE_S3_SECRET_KEY", "DDCORE_SMTP_PASSWORD", "DDCORE_DSN"} {
		if _, leaked := env.Values[k]; leaked {
			t.Errorf("%s was written with its value", k)
		}
		found := false
		for _, s := range env.Secrets {
			found = found || s == k
		}
		if !found {
			t.Errorf("%s is not listed as a secret to provision", k)
		}
	}
	if env.Values["DDCORE_URL"] != "https://example.com" {
		t.Errorf("non-secret values are kept: %v", env.Values)
	}
}

func TestOldestArchives(t *testing.T) {
	got := oldest([]string{"ddcore-20260103-000000.tar", "ddcore-20260101-000000.tar", "ddcore-20260102-000000.tar"}, 2)
	if len(got) != 1 || got[0] != "ddcore-20260101-000000.tar" {
		t.Errorf("oldest = %v", got)
	}
}

// The whole round trip, against real PostgreSQL and pg_dump: back up a site
// with a user and stored files, restore it into an empty database and another
// data directory, and check what a drill checks.
func TestBackupRestoreRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("pg_dump"); err != nil {
		t.Skip("pg_dump not on PATH")
	}
	if _, err := exec.LookPath("pg_restore"); err != nil {
		t.Skip("pg_restore not on PATH")
	}
	base := os.Getenv("DDCORE_TEST_DSN")
	if base == "" {
		base = "postgres://ddcore:ddcore@localhost:5455/ddcore_test?sslmode=disable"
	}
	ctx := context.Background()
	src, dst := dsnWithDB(base, "ddcore_test_backup_src"), dsnWithDB(base, "ddcore_test_backup_dst")
	admin, err := db.Open(ctx, dsnWithDB(base, "postgres"))
	if err != nil {
		t.Skipf("postgres unavailable: %v", err)
	}
	for _, name := range []string{"ddcore_test_backup_src", "ddcore_test_backup_dst"} {
		admin.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		if _, err := admin.Pool.Exec(ctx, "CREATE DATABASE "+name); err != nil {
			t.Fatal(err)
		}
	}
	admin.Close()
	work := t.TempDir()
	t.Setenv("DDCORE_SMTP_PASSWORD", "never-in-the-archive")

	// the source site
	t.Setenv("DDCORE_DSN", src)
	t.Setenv("DDCORE_DATA_DIR", filepath.Join(work, "src"))
	e, _, err := load(false, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	if err := e.SetPassword(ctx, "Administrator", "restore-drill-1"); err != nil {
		t.Fatal(err)
	}
	for key, body := range map[string]string{"public/a.txt": "hello", "private/b.pdf": "%PDF"} {
		if err := e.Storage().Put(ctx, key, strings.NewReader(body), int64(len(body)), ""); err != nil {
			t.Fatal(err)
		}
	}
	// a stray dotfile is no stored file: backup leaves it out, or restore
	// would refuse the whole archive
	if err := os.WriteFile(filepath.Join(work, "src", "files", "public", ".DS_Store"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	e.DB.Close()
	archive := filepath.Join(work, "site.tar")
	if err := cmdBackup([]string{"--out", archive}); err != nil {
		t.Fatalf("backup: %v", err)
	}
	if b, _ := os.ReadFile(archive); strings.Contains(string(b), "never-in-the-archive") {
		t.Fatal("a secret value reached the archive")
	}

	// the target: empty, elsewhere
	t.Setenv("DDCORE_DSN", dst)
	t.Setenv("DDCORE_DATA_DIR", filepath.Join(work, "dst"))
	t.Setenv("DDCORE_SMOKE_PASSWORD", "restore-drill-1")
	if err := cmdRestore([]string{archive, "--smoke", "--smoke-user", "Administrator"}); err != nil {
		t.Fatalf("restore: %v", err)
	}
	store := storage.NewLocal(filepath.Join(work, "dst", "files"))
	if b, err := storage.ReadAll(ctx, store, "private/b.pdf"); err != nil || string(b) != "%PDF" {
		t.Fatalf("restored file: %q %v", b, err)
	}
	e, _, err = load(false, false)
	if err != nil {
		t.Fatal(err)
	}
	defer e.DB.Close()
	if st := e.Maintenance(ctx); !st.Enabled {
		t.Error("a restored site stays paused until someone reopens it")
	}

	// a second restore onto the same database needs --force
	if err := cmdRestore([]string{archive}); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("restore over an existing site: %v", err)
	}

	// a --force restore that fails leaves the old site as it was, open again
	if _, err := e.SetMaintenance(ctx, false, "", "t"); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DB.Pool.Exec(ctx, `CREATE VIEW restore_blocker AS SELECT * FROM ddcore_session`); err != nil {
		t.Fatal(err)
	}
	if err := cmdRestore([]string{archive, "--force"}); err == nil {
		t.Error("a restore whose tables cannot be dropped should fail")
	}
	var paused bool
	if err := e.DB.Pool.QueryRow(ctx, `SELECT enabled FROM ddcore_maintenance WHERE id = 1`).Scan(&paused); err != nil || paused {
		t.Errorf("after a failed --force restore the target is paused=%v (%v)", paused, err)
	}
	e.DB.Pool.Exec(ctx, `DROP VIEW restore_blocker`)

	// a damaged archive is refused before anything is touched
	damaged := filepath.Join(work, "damaged.tar")
	if err := rewriteEntry(archive, damaged, "files/public/a.txt", "HELLO"); err != nil {
		t.Fatal(err)
	}
	if err := cmdRestore([]string{damaged, "--verify-only"}); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Errorf("damaged archive: %v", err)
	}
}

func dsnWithDB(dsn, name string) string {
	i := strings.LastIndex(dsn, "/")
	q := ""
	if j := strings.Index(dsn[i:], "?"); j >= 0 {
		q = dsn[i+j:]
	}
	return dsn[:i+1] + name + q
}

// rewriteEntry copies a tar, replacing one entry's content with a same-sized
// body, which only the checksum can catch.
func rewriteEntry(in, out, name, body string) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	o, err := os.Create(out)
	if err != nil {
		return err
	}
	defer o.Close()
	tr, tw := tar.NewReader(f), tar.NewWriter(o)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if h.Name == name {
			_, err = io.WriteString(tw, body)
		} else {
			_, err = io.Copy(tw, tr)
		}
		if err != nil {
			return err
		}
	}
	return tw.Close()
}

// Nothing after the database step can be undone. A failure there leaves a
// restored database behind a pause, and the operator has to be told: the
// printed error alone reads like "nothing happened".
func TestRestoreWarnsWhenItStopsAfterTheDatabase(t *testing.T) {
	if got := afterDatabaseWarning(false, errors.New("archive is corrupt")); got != "" {
		t.Errorf("a failure before the database step needs no warning, got %q", got)
	}
	if got := afterDatabaseWarning(true, nil); got != "" {
		t.Errorf("a run that worked needs no warning, got %q", got)
	}
	got := afterDatabaseWarning(true, errors.New("storage unreachable"))
	if !strings.Contains(got, "maintenance off") || !strings.Contains(got, "database") {
		t.Errorf("the warning does not say what was left behind: %q", got)
	}
}
