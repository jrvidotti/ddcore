package engine

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/testdb"
)

// Port 1 refuses at once, so the attempt fails fast.
const unreachableDSN = "postgres://ddcore:ddcore@127.0.0.1:1/ddcore?connect_timeout=2"

func TestDeferDBBootsWithoutTheDatabaseAndConnectsLater(t *testing.T) {
	ctx := context.Background()
	if _, err := New(ctx, Config{DSN: unreachableDSN, LogOut: io.Discard}); err == nil {
		t.Fatal("without DeferDB an unreachable database must stop New")
	}
	e, err := New(ctx, Config{DSN: unreachableDSN, DeferDB: true, LogOut: io.Discard})
	if err != nil {
		t.Fatalf("DeferDB: %v", err)
	}
	if e.DB != nil {
		t.Fatal("DB set without a database")
	}
	if err := e.Connect(ctx); err == nil || e.DB != nil {
		t.Fatalf("Connect to an unreachable database: err=%v DB=%v", err, e.DB)
	}

	// Postgres comes up: the same engine connects without being rebuilt.
	dsn := testdb.WithDatabase(testDSN, testdb.Database(testDSN)+"_defer")
	err = testdb.Empty(ctx, dsn)
	if errors.Is(err, testdb.ErrUnavailable) && os.Getenv("DDCORE_TEST_DSN") == "" {
		t.Skip(err)
	}
	if err != nil {
		t.Fatal(err)
	}
	e.Cfg.DSN = dsn
	if err := e.Connect(ctx); err != nil {
		t.Fatalf("Connect once the database answers: %v", err)
	}
	t.Cleanup(func() { e.DB.Close() })
	first := e.DB
	if err := e.Connect(ctx); err != nil || e.DB != first {
		t.Fatalf("a second Connect must keep the pool it has: err=%v", err)
	}
}

// An app that imports another app's module depends on it: `requires` has to
// say so, and with it the load order follows whatever ddcore.json lists (#47).
func TestCrossAppImportNeedsRequires(t *testing.T) {
	root := t.TempDir()
	write := func(rel, src string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	manifest := func(name, extra string) string {
		return `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "` + name + `", title: "` + name + `"` + extra + ` });`
	}
	write("lib/ddcore.app.ts", manifest("lib", ""))
	write("lib/services/thing.ts", `export const gateway = { call: () => "real" };`)
	write("site/ddcore.app.ts", manifest("site", ""))
	write("site/services/use.ts", `import { gateway } from "../../lib/services/thing";
export function use() { return gateway.call(); }`)
	// the importer first, as a site's ddcore.json would list its own app
	cfg := Config{DSN: unreachableDSN, DeferDB: true, LogOut: io.Discard, Apps: []js.App{
		{Name: "site", Dir: filepath.Join(root, "site")}, {Name: "lib", Dir: filepath.Join(root, "lib")},
	}}
	ctx := context.Background()

	_, err := New(ctx, cfg)
	if err == nil || !strings.Contains(err.Error(), `app site imports a module of app lib: add "lib" to requires`) {
		t.Fatalf("expected the load to ask for requires, got %v", err)
	}

	write("site/ddcore.app.ts", manifest("site", `, requires: ["lib"]`))
	e, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("with requires declared: %v", err)
	}
	if got := strings.Join(e.AppOrder(), ","); got != "core,lib,site" {
		t.Fatalf("load order = %s, expected core,lib,site", got)
	}
	if got := e.SiteTitle(); got != "site" {
		t.Fatalf("SiteTitle() = %q: the app that imports the library names the site", got)
	}
}
