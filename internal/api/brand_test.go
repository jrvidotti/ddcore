package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// setupBrand is the demo app with a theme and an SVG logo (#128), loaded the
// way a site loads it: through defineApp and Engine.Load.
func setupBrand(t *testing.T) *env {
	t.Helper()
	dir := testApp(t)
	manifest := `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"],
  desk: {
    logo: { image: "assets/mark.svg" },
    theme: {
      colors: { primary: "#7c2d2d", bg: "#f4efe4" },
      fonts: { heading: "'Cormorant Garamond', Georgia, serif", stylesheet: "https://fonts.googleapis.com/css2?family=Cormorant+Garamond&display=swap" },
    },
  } });`
	for rel, src := range map[string]string{"ddcore.app.ts": manifest, "assets/mark.svg": `<svg xmlns="http://www.w3.org/2000/svg"><circle r="4"/></svg>`} {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return setupApp(t, dir)
}

// TestBrandShell: the shell already wears the site's look, so the sign-in
// page's first paint is the app's and not the desk's blue.
func TestBrandShell(t *testing.T) {
	x := setupBrand(t)
	shell := `<!doctype html><html lang="en"><head><title>ddcore</title><link rel="stylesheet" href="/_app/app.css"></head><body></body></html>`
	x.s.Desk = fstest.MapFS{"index.html": {Data: []byte(shell)}}
	r := req("Guest")
	r.URL.Path = "/login"
	w := httptest.NewRecorder()
	x.s.deskHandler(w, r)
	got := w.Body.String()
	for _, want := range []string{
		"<title>Demo</title>",
		`<link rel="icon" href="/assets/site/favicon?v=`,
		`<meta name="theme-color" content="#7c2d2d">`,
		`<link id="ddcore-fonts" rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Cormorant+Garamond&amp;display=swap">`,
		`<style id="ddcore-brand">:root:root{--primary:#7c2d2d;`,
		"--font-heading:'Cormorant Garamond', Georgia, serif",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("shell is missing %q:\n%s", want, got)
		}
	}
	// after the desk's stylesheet, before the end of the head
	if strings.Index(got, "/_app/app.css") > strings.Index(got, "ddcore-brand") || !strings.Contains(got, "</style></head>") {
		t.Errorf("the theme must follow the desk's stylesheet, at the end of the head:\n%s", got)
	}

	// boot repeats it for a visitor, with the logo's URL
	b := x.call("GET", "/api/boot", nil, "")
	site := b.Body["data"].(map[string]any)["site"].(map[string]any)
	brand, _ := site["brand"].(map[string]any)
	if css, _ := brand["css"].(string); !strings.Contains(css, "--primary:#7c2d2d") {
		t.Errorf("boot site.brand.css = %v", brand["css"])
	}
	logo, _ := brand["logoImage"].(string)
	if !strings.HasPrefix(logo, "/assets/site/logo?v=") || brand["themeColor"] != "#7c2d2d" || brand["stylesheet"] == nil {
		t.Errorf("boot site.brand = %v", brand)
	}
	if site["logo"] != "" {
		t.Errorf("site.logo = %v, expected no letter: the app's logo is an image", site["logo"])
	}

	// the logo is public, served as what it is, cached for good at its version
	res := x.call("GET", logo, nil, "")
	if res.Status != 200 || !strings.Contains(string(res.Raw), "<circle") {
		t.Fatalf("logo: %d %s", res.Status, res.Raw)
	}
	resp, err := http.Get(x.ts.URL + logo)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	h := resp.Header
	if h.Get("Content-Type") != "image/svg+xml" || h.Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(h.Get("Content-Security-Policy"), "sandbox") {
		t.Errorf("logo headers: %v", h)
	}
	// the favicon is the logo when none is declared
	if fav := x.call("GET", "/assets/site/favicon", nil, ""); fav.Status != 200 || !strings.Contains(string(fav.Raw), "<circle") {
		t.Errorf("favicon: %d %s", fav.Status, fav.Raw)
	}
	// the route names a role, not a path
	if res := x.call("GET", "/assets/site/ddcore.app.ts", nil, ""); res.Status == 200 && strings.Contains(string(res.Raw), "defineApp") {
		t.Errorf("/assets/site/ served an app file: %s", res.Raw)
	}
}

// TestBrandDefault: a site without a theme keeps the desk's look; it still
// gets a favicon — its letter square — and no logo.
func TestBrandDefault(t *testing.T) {
	x := setup(t)
	shell := `<!doctype html><html lang="en"><head><title>ddcore</title></head><body></body></html>`
	x.s.Desk = fstest.MapFS{"index.html": {Data: []byte(shell)}}
	r := req("Guest")
	r.URL.Path = "/"
	w := httptest.NewRecorder()
	x.s.deskHandler(w, r)
	got := w.Body.String()
	if !strings.Contains(got, "<title>Demo</title>") || !strings.Contains(got, `rel="icon"`) {
		t.Errorf("shell: %s", got)
	}
	if strings.Contains(got, "ddcore-brand") || strings.Contains(got, "theme-color") || strings.Contains(got, "ddcore-fonts") {
		t.Errorf("a site without a theme got one: %s", got)
	}
	if res := x.call("GET", "/assets/site/logo", nil, ""); res.Status != 404 {
		t.Errorf("logo without one declared: %d", res.Status)
	}
	fav := x.call("GET", "/assets/site/favicon", nil, "")
	if fav.Status != 200 || !strings.Contains(string(fav.Raw), `fill="#2563eb"`) || !strings.Contains(string(fav.Raw), ">D</text>") {
		t.Errorf("generated favicon: %d %s", fav.Status, fav.Raw)
	}
}
