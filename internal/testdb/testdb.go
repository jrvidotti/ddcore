// Package testdb gives the Postgres-backed test suites their databases.
//
// Two things make those suites slow or flaky when every test builds its own
// database from scratch:
//
//   - A full migrate per test. Here a database is migrated once per process
//     for each distinct app (the template), and each test gets a copy of it
//     with CREATE DATABASE ... TEMPLATE, which is a file copy.
//   - Fixed database names. Two test runs at once (a terminal and an editor,
//     say) dropped each other's databases mid-test. Names now carry the
//     process id, and a run drops what it created, plus whatever a run that
//     is no longer alive left behind.
//
// It is only imported from _test.go files.
package testdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// DefaultDSN is the test cluster when DDCORE_TEST_DSN is not set: the
// `ddcore-pg` container from docker-compose.yml.
const DefaultDSN = "postgres://ddcore:ddcore@localhost:5455/ddcore_test?sslmode=disable"

// ErrUnavailable wraps a failure to reach the cluster at all, which a suite
// skips on unless DDCORE_TEST_DSN was set explicitly.
var ErrUnavailable = errors.New("postgres unavailable")

// pidTag is what makes this process's database names its own.
var pidTag = "_p" + strconv.Itoa(os.Getpid())

// BaseDSN is DDCORE_TEST_DSN (or DefaultDSN) with this process's tag on the
// database name: ddcore_test becomes ddcore_test_p12345.
func BaseDSN() string {
	base := os.Getenv("DDCORE_TEST_DSN")
	if base == "" {
		base = DefaultDSN
	}
	return WithDatabase(base, Database(base)+pidTag)
}

// Database is the database name in a DSN.
func Database(dsn string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(u.Path, "/")
}

// WithDatabase returns dsn pointing at another database.
func WithDatabase(dsn, name string) string {
	u, err := url.Parse(dsn)
	if err != nil {
		return dsn
	}
	u.Path = "/" + name
	return u.String()
}

// AdminDSN is dsn pointing at the `postgres` maintenance database, from which
// others are created and dropped.
func AdminDSN(dsn string) string { return WithDatabase(dsn, "postgres") }

var (
	mu        sync.Mutex
	templates = map[string]bool{} // template name -> migrated in this process
	created   = map[string]bool{} // every database this process created
)

// Key identifies what a template holds: the files of the app directories
// (relative path and contents, never the temporary directory they sit in)
// and any settings that change what migrate writes.
func Key(dirs []string, settings ...string) string {
	h := sha256.New()
	for _, dir := range dirs {
		var files []string
		filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				files = append(files, p)
			}
			return nil
		})
		sort.Strings(files)
		fmt.Fprintf(h, "dir\x00")
		for _, p := range files {
			rel, _ := filepath.Rel(dir, p)
			b, _ := os.ReadFile(p)
			fmt.Fprintf(h, "%s\x00%d\x00", filepath.ToSlash(rel), len(b))
			h.Write(b)
		}
	}
	for _, s := range settings {
		fmt.Fprintf(h, "set\x00%s\x00", s)
	}
	return hex.EncodeToString(h.Sum(nil))[:10]
}

// Fresh makes dsn's database a new copy of the template for key, migrating
// that template first if this process has not yet. migrate receives the
// template's DSN and must leave no connection open to it when it returns.
func Fresh(ctx context.Context, dsn, key string, migrate func(templateDSN string) error) error {
	mu.Lock()
	defer mu.Unlock()
	name := Database(dsn)
	// named after the run, not the database: tests that each use their own
	// database but load the same app share one template
	tpl := name + "_t" + key
	if i := strings.Index(name, pidTag); i >= 0 {
		tpl = name[:i+len(pidTag)] + "_t" + key
	}
	admin, err := pgx.Connect(ctx, AdminDSN(dsn))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer admin.Close(ctx)
	if !templates[tpl] {
		if err := recreate(ctx, admin, tpl, ""); err != nil {
			return err
		}
		if err := migrate(WithDatabase(dsn, tpl)); err != nil {
			// a half-migrated template must not be copied by the next test
			admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(tpl)+" WITH (FORCE)")
			return err
		}
		templates[tpl] = true
	}
	return recreate(ctx, admin, name, tpl)
}

// Empty makes dsn's database a new, empty one, for a test that migrates it
// itself (through the binary, say).
func Empty(ctx context.Context, dsn string) error {
	mu.Lock()
	defer mu.Unlock()
	admin, err := pgx.Connect(ctx, AdminDSN(dsn))
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer admin.Close(ctx)
	return recreate(ctx, admin, Database(dsn), "")
}

// recreate drops name and creates it again, empty or as a copy of template.
func recreate(ctx context.Context, admin *pgx.Conn, name, template string) error {
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(name)+" WITH (FORCE)"); err != nil {
		return err
	}
	q := "CREATE DATABASE " + ident(name)
	if template != "" {
		q += " TEMPLATE " + ident(template)
	}
	created[name] = true
	// A copy needs the template to have no connections, and a backend the
	// migrate just closed can take a moment to exit.
	for i := 0; ; i++ {
		_, err := admin.Exec(ctx, q)
		var pe *pgconn.PgError
		if err == nil || i >= 50 || !errors.As(err, &pe) || pe.Code != "55006" {
			return err
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// Main runs a suite's tests between a sweep of databases left by dead runs and
// a drop of everything this run created. Use it from TestMain:
//
//	func TestMain(m *testing.M) { os.Exit(testdb.Main(m)) }
func Main(m *testing.M) int {
	dsn := BaseDSN()
	ReapStale(dsn)
	code := m.Run()
	DropOwn(dsn)
	return code
}

// DropOwn drops every database this process created, templates included.
func DropOwn(dsn string) {
	mu.Lock()
	defer mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, AdminDSN(dsn))
	if err != nil {
		return
	}
	defer admin.Close(ctx)
	for name := range created {
		admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(name)+" WITH (FORCE)")
	}
	created = map[string]bool{}
	templates = map[string]bool{}
}

var tagRe = regexp.MustCompile(`_p(\d+)(_|$)`)

// ReapStale drops the databases a test run tagged with its pid and left
// behind because it was killed before DropOwn ran. A pid still alive is
// another run in progress, and its databases are left alone.
func ReapStale(dsn string) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, AdminDSN(dsn))
	if err != nil {
		return
	}
	defer admin.Close(ctx)
	base := strings.TrimSuffix(Database(dsn), pidTag)
	rows, err := admin.Query(ctx, "SELECT datname FROM pg_database WHERE datname LIKE $1", base+"\\_p%")
	if err != nil {
		return
	}
	var stale []string
	for rows.Next() {
		var name string
		if rows.Scan(&name) != nil {
			continue
		}
		m := tagRe.FindStringSubmatch(strings.TrimPrefix(name, base))
		if m == nil {
			continue
		}
		if pid, _ := strconv.Atoi(m[1]); pid != os.Getpid() && !alive(pid) {
			stale = append(stale, name)
		}
	}
	rows.Close()
	for _, name := range stale {
		admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident(name)+" WITH (FORCE)")
	}
}

func alive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	return err == nil || errors.Is(err, syscall.EPERM)
}

func ident(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
