package engine

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// wwwSite loads two apps whose manifests carry the given `www` blocks, without
// a database: the checks run in Load, before anything touches one.
func wwwSite(t *testing.T, a, b string) (*Engine, string, error) {
	t.Helper()
	root := t.TempDir()
	for _, app := range []struct{ name, www string }{{"shop", a}, {"blog", b}} {
		dir := filepath.Join(root, app.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		extra := ""
		if app.www != "" {
			extra = ", www: " + app.www
		}
		src := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "` + app.name + `", title: "` + app.name + `"` + extra + ` });`
		if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cfg := Config{DSN: unreachableDSN, DeferDB: true, LogOut: io.Discard, Apps: []js.App{
		{Name: "shop", Dir: filepath.Join(root, "shop")}, {Name: "blog", Dir: filepath.Join(root, "blog")},
	}}
	e, err := New(context.Background(), cfg)
	return e, root, err
}

func TestWWWDeclaredSitesLoad(t *testing.T) {
	// neither directory exists: the build may come after the app
	e, root, err := wwwSite(t, `{ "/r": "checkout/build" }`,
		`{ "/blog": { dir: "site", fallback: null, frame: true }, "/docs": { dir: "docs/out", fallback: "404.html" } }`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	st := e.Current()
	r, ok := st.WWW("r")
	if !ok {
		t.Fatal("/r is not served")
	}
	want := WWWSite{Prefix: "r", App: "shop", Dir: filepath.Join(root, "shop", "checkout", "build"), Fallback: "index.html"}
	if r != want {
		t.Fatalf("/r = %+v, want %+v", r, want)
	}
	b, _ := st.WWW("blog")
	if b.Fallback != "" || !b.Frame || b.App != "blog" {
		t.Fatalf("/blog = %+v: fallback null means none, frame true opts out of DENY", b)
	}
	if d, _ := st.WWW("docs"); d.Fallback != "404.html" || d.Frame {
		t.Fatalf("/docs = %+v", d)
	}
	if _, ok := st.WWW("app"); ok {
		t.Fatal("an undeclared prefix resolved")
	}
}

func TestWWWRefusesBadDeclarations(t *testing.T) {
	for _, c := range []struct{ name, a, b, want string }{
		{"reserved", `{ "/api": "build" }`, "", `"/api" is reserved`},
		{"reserved desk assets", `{ "/_app": "build" }`, "", `"/_app"`},
		{"reserved probe", `{ "/healthz": "build" }`, "", `"/healthz" is reserved`},
		{"clash", `{ "/r": "build" }`, `{ "/r": "out" }`, `taken by app "shop"`},
		{"two segments", `{ "/r/x": "build" }`, "", `"/r/x" must be a single lowercase path segment`},
		{"no slash", `{ "r": "build" }`, "", `"r" must be a single lowercase path segment`},
		{"upper case", `{ "/R": "build" }`, "", `must be a single lowercase path segment`},
		{"escapes", `{ "/r": "../blog" }`, "", `must stay inside the app`},
		{"absolute", `{ "/r": "/etc" }`, "", `must stay inside the app`},
		{"app root", `{ "/r": "." }`, "", `must stay inside the app`},
		{"no dir", `{ "/r": {} }`, "", `needs a dir`},
		{"fallback escapes", `{ "/r": { dir: "build", fallback: "../x.html" } }`, "", `fallback "../x.html"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := wwwSite(t, c.a, c.b)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error containing %q, got %v", c.want, err)
			}
		})
	}
}

// One site may claim the site root (#169); without the claim, / stays the desk's.
func TestWWWRootClaim(t *testing.T) {
	e, _, err := wwwSite(t, `{ "/r": "build", "/atleta": { dir: "client/athlete/build", root: true } }`, `{ "/blog": "site" }`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	site, ok := e.Current().WWWRoot()
	if !ok || site.Prefix != "atleta" || site.App != "shop" || !site.Root {
		t.Fatalf("WWWRoot = %+v, %v", site, ok)
	}
	if r, _ := e.Current().WWW("r"); r.Root {
		t.Fatal("/r claims the root without saying so")
	}
	e, _, err = wwwSite(t, `{ "/r": { dir: "build", root: false } }`, `{ "/blog": "site" }`)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if site, ok := e.Current().WWWRoot(); ok {
		t.Fatalf("no site claims the root, got %+v", site)
	}
}

func TestWWWRootRefusesTwoClaims(t *testing.T) {
	for _, c := range []struct{ name, a, b, want string }{
		{"two apps", `{ "/r": { dir: "build", root: true } }`, `{ "/blog": { dir: "site", root: true } }`,
			`app "blog": www "/blog" claims the site root, already claimed by app "shop"'s www "/r"`},
		{"one app", `{ "/a": { dir: "a", root: true }, "/b": { dir: "b", root: true } }`, "",
			`app "shop": www "/b" claims the site root, already claimed by app "shop"'s www "/a"`},
		{"not a boolean", `{ "/r": { dir: "build", root: "yes" } }`, "", `"/r": root must be true or false`},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, _, err := wwwSite(t, c.a, c.b)
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("expected an error containing %q, got %v", c.want, err)
			}
		})
	}
}
