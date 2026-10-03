package api

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// setupWWW is the test app with two static sites: /r, a SPA with the default
// fallback, and /plain, with no fallback and allowed in a frame. The files are
// written after setup on purpose: the template database is keyed on the app's
// contents, and the files are what each test varies.
func setupWWW(t *testing.T) (*env, string) {
	t.Helper()
	dir := testApp(t)
	manifest := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"],
  www: { "/r": "checkout/build", "/plain": { dir: "plain", fallback: null, frame: true } } });`
	if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	x := setupApp(t, dir)
	w := func(rel, src string) {
		p := filepath.Join(dir, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("checkout/build/index.html", "<html>checkout shell</html>")
	w("checkout/build/style.css", "body{color:red}")
	w("checkout/build/_app/immutable/entry.abc123.js", "console.log(1)")
	w("checkout/build/help/index.html", "<html>help page</html>")
	w("checkout/secret.txt", "outside the build")
	w("plain/hello.txt", "hello")
	return x, dir
}

// get fetches a path without following redirects, so a 308 is seen as one.
func (x *env) get(method, path string) resp {
	x.t.Helper()
	req, _ := http.NewRequest(method, x.ts.URL+path, nil)
	c := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := c.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	var b strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := res.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return resp{Status: res.StatusCode, Raw: b.String(), Header: res.Header}
}

func TestWWWServesTheAppSite(t *testing.T) {
	x, _ := setupWWW(t)
	r := x.get("GET", "/r/style.css")
	if r.Status != 200 || r.Raw != "body{color:red}" {
		t.Fatalf("style.css: %d %q", r.Status, r.Raw)
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Fatalf("Content-Type = %q", ct)
	}
	if r := x.get("HEAD", "/r/style.css"); r.Status != 200 || r.Raw != "" {
		t.Fatalf("HEAD: %d %q", r.Status, r.Raw)
	}
	// the site's root and a directory serve their index.html
	if r := x.get("GET", "/r/"); r.Status != 200 || r.Raw != "<html>checkout shell</html>" {
		t.Fatalf("/r/: %d %q", r.Status, r.Raw)
	}
	if r := x.get("GET", "/r/help/"); r.Status != 200 || r.Raw != "<html>help page</html>" {
		t.Fatalf("/r/help/: %d %q", r.Status, r.Raw)
	}
	if r := x.get("GET", "/r/help"); r.Status != 200 || r.Raw != "<html>help page</html>" {
		t.Fatalf("/r/help: %d %q", r.Status, r.Raw)
	}
	// a client-side route gets the shell
	r = x.get("GET", "/r/ABC123?x=1")
	if r.Status != 200 || r.Raw != "<html>checkout shell</html>" {
		t.Fatalf("fallback: %d %q", r.Status, r.Raw)
	}
	if ct := r.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("fallback Content-Type = %q", ct)
	}
	// without a fallback, a missing file is missing
	if r := x.get("GET", "/plain/hello.txt"); r.Status != 200 || r.Raw != "hello" {
		t.Fatalf("/plain/hello.txt: %d %q", r.Status, r.Raw)
	}
	if r := x.get("GET", "/plain/nope"); r.Status != 404 {
		t.Fatalf("/plain/nope: %d %q", r.Status, r.Raw)
	}
}

func TestWWWRefusesTraversal(t *testing.T) {
	x, _ := setupWWW(t)
	for _, p := range []string{"/r/../secret.txt", "/r/%2e%2e/secret.txt", "/r/..%2fsecret.txt", "/r/a/../../secret.txt", "/plain/../checkout/secret.txt"} {
		r := x.get("GET", p)
		if r.Status != 404 || strings.Contains(r.Raw, "outside the build") || strings.Contains(r.Raw, "checkout shell") {
			t.Fatalf("%s: %d %q", p, r.Status, r.Raw)
		}
	}
}

func TestWWWRedirectsThePrefixToItsSlash(t *testing.T) {
	x, _ := setupWWW(t)
	r := x.get("GET", "/r?code=1")
	if r.Status != 308 || r.Header.Get("Location") != "/r/?code=1" {
		t.Fatalf("/r: %d Location=%q", r.Status, r.Header.Get("Location"))
	}
}

func TestWWWHeaders(t *testing.T) {
	x, _ := setupWWW(t)
	for _, c := range []struct{ path, cache string }{
		{"/r/", "no-cache"},
		{"/r/some/route", "no-cache"},
		{"/r/style.css", "public, max-age=300"},
		{"/r/_app/immutable/entry.abc123.js", "public, max-age=31536000, immutable"},
	} {
		r := x.get("GET", c.path)
		if r.Status != 200 {
			t.Fatalf("%s: %d", c.path, r.Status)
		}
		if got := r.Header.Get("Cache-Control"); got != c.cache {
			t.Fatalf("%s: Cache-Control = %q, want %q", c.path, got, c.cache)
		}
		if r.Header.Get("X-Content-Type-Options") != "nosniff" || r.Header.Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
			t.Fatalf("%s: security headers missing: %v", c.path, r.Header)
		}
		if r.Header.Get("X-Frame-Options") != "DENY" {
			t.Fatalf("%s: X-Frame-Options = %q", c.path, r.Header.Get("X-Frame-Options"))
		}
	}
	// frame: true lets another page embed the site
	if r := x.get("GET", "/plain/hello.txt"); r.Header.Get("X-Frame-Options") != "" || r.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("/plain: %v", r.Header)
	}
}

func TestWWWOnlyReads(t *testing.T) {
	x, _ := setupWWW(t)
	for _, m := range []string{"POST", "PUT", "DELETE"} {
		r := x.get(m, "/r/style.css")
		if r.Status != 405 || r.Header.Get("Allow") != "GET, HEAD" {
			t.Fatalf("%s: %d Allow=%q", m, r.Status, r.Header.Get("Allow"))
		}
	}
}

func TestWWWLeavesTheDeskAlone(t *testing.T) {
	x, _ := setupWWW(t)
	x.s.Desk = fstest.MapFS{"index.html": {Data: []byte(`<html lang="en">desk</html>`)}}
	for _, p := range []string{"/", "/app", "/app/pessoa", "/login", "/rr", "/rx/y"} {
		if r := x.get("GET", p); r.Status != 200 || !strings.Contains(r.Raw, "desk") {
			t.Fatalf("%s: %d %q", p, r.Status, r.Raw)
		}
	}
	if r := x.get("GET", "/api/nope"); r.Status != 404 || strings.Contains(r.Raw, "desk") {
		t.Fatalf("/api/nope: %d %q", r.Status, r.Raw)
	}
}

// The directory is read on each request: a rebuild of the app's site shows
// without a reload, as the dev server's other files do.
func TestWWWSeesAFileChangedOnDisk(t *testing.T) {
	x, dir := setupWWW(t)
	if r := x.get("GET", "/r/new.txt"); r.Raw == "fresh" {
		t.Fatal("served before it existed")
	}
	if err := os.WriteFile(filepath.Join(dir, "checkout/build/new.txt"), []byte("fresh"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := x.get("GET", "/r/new.txt"); r.Status != 200 || r.Raw != "fresh" {
		t.Fatalf("new file: %d %q", r.Status, r.Raw)
	}
	if err := os.WriteFile(filepath.Join(dir, "checkout/build/new.txt"), []byte("fresher"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := x.get("GET", "/r/new.txt"); r.Raw != "fresher" {
		t.Fatalf("changed file: %q", r.Raw)
	}
}

// A guest gets the site even where the desk would ask for a sign-in.
func TestWWWIsPublic(t *testing.T) {
	x, _ := setupWWW(t)
	if r := x.call("GET", "/r/style.css", nil, "token:"+x.apiKey("ze@x.com")); r.Status != 200 {
		t.Fatalf("signed in: %d", r.Status)
	}
	if r := x.call("GET", "/r/style.css", nil, ""); r.Status != 200 {
		t.Fatalf("guest: %d", r.Status)
	}
}

// The router is built once; a prefix a reload adds or drops is still seen,
// because the site is looked up on the state of each request.
func TestWWWFollowsAReload(t *testing.T) {
	x, dir := setupWWW(t)
	manifest := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"], www: { "/pay": "checkout/build" } });`
	if err := os.WriteFile(filepath.Join(dir, "ddcore.app.ts"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := x.e.Load(); err != nil {
		t.Fatal(err)
	}
	if r := x.get("GET", "/pay/style.css"); r.Status != 200 || r.Raw != "body{color:red}" {
		t.Fatalf("/pay after reload: %d %q", r.Status, r.Raw)
	}
	if r := x.get("GET", "/r/style.css"); r.Status != 404 {
		t.Fatalf("/r after it was dropped: %d %q", r.Status, r.Raw)
	}
}
