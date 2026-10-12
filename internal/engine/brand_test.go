package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/js"
)

// brandState is state() with each app given a directory, which buildBrand
// opens to find the logo files.
func brandState(t *testing.T, names ...string) (*State, map[string]string) {
	t.Helper()
	st := &State{Snap: &Snapshot{Apps: map[string]*AppMeta{}}}
	dirs := map[string]string{}
	for _, n := range names {
		dir := t.TempDir()
		dirs[n] = dir
		st.Apps = append(st.Apps, js.App{Name: n, Dir: dir})
		st.Snap.Apps[n] = &AppMeta{Name: n, Title: n, Dir: dir}
	}
	return st, dirs
}

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBrandPrecedence: the theme and the logo are the site's app's, ahead of
// a library it requires, each key on its own — as with `home`.
func TestBrandPrecedence(t *testing.T) {
	st, dirs := brandState(t, "core", "lib", "site")
	st.Snap.Apps["site"].Requires = []string{"lib"}
	writeFile(t, dirs["lib"], "assets/lib.svg", "<svg/>")
	writeFile(t, dirs["site"], "assets/mark.svg", "<svg>site</svg>")
	st.Snap.Apps["lib"].Desk = map[string]any{
		"theme": map[string]any{"colors": map[string]any{"primary": "#123456"}},
		"logo":  map[string]any{"image": "assets/lib.svg"},
	}
	st.Snap.Apps["site"].Desk = map[string]any{
		"theme": map[string]any{"colors": map[string]any{"primary": "#7c2d2d"}},
		"logo":  map[string]any{"image": "assets/mark.svg"},
	}
	b, err := buildBrand(st.Apps, st.Snap)
	if err != nil {
		t.Fatal(err)
	}
	if b.ThemeApp != "site" || b.Theme.Colors.Primary != "#7c2d2d" {
		t.Errorf("theme from %q (%q), expected the site app's", b.ThemeApp, b.Theme.Colors.Primary)
	}
	if b.Logo == nil || b.Logo.App != "site" || b.Logo.Path != filepath.Join("assets", "mark.svg") || b.Logo.Version == "" {
		t.Errorf("logo = %+v, expected the site app's file with a version", b.Logo)
	}
	if b.Favicon != nil {
		t.Errorf("favicon = %+v, expected none declared", b.Favicon)
	}

	// a letter decides as much as an image: the library's image does not
	// replace the site's letter, and SiteDesk reports the letter
	st.Snap.Apps["site"].Desk = map[string]any{"logo": "O"}
	if b, err = buildBrand(st.Apps, st.Snap); err != nil {
		t.Fatal(err)
	}
	if b.Logo != nil {
		t.Errorf("logo = %+v, expected the site's letter to win over the library's image", b.Logo)
	}
	if b.ThemeApp != "lib" {
		t.Errorf("theme from %q, expected the library's when the site declares none", b.ThemeApp)
	}

	// and an image decides as much as a letter: SiteDesk has no letter to give
	st.Snap.Apps["site"].Desk = map[string]any{"logo": map[string]any{"image": "assets/mark.svg"}}
	st.Snap.Apps["lib"].Desk = map[string]any{"logo": "L"}
	if got := st.SiteDesk("logo"); got != "" {
		t.Errorf("SiteDesk(logo) = %q, expected no letter: the site's app declares an image", got)
	}
}

// TestBrandCSS: what the theme declares becomes the desk's tokens, on a
// selector that outranks the desk's own :root; primaryHover and onPrimary
// follow primary when left out.
func TestBrandCSS(t *testing.T) {
	if css := (&Brand{}).CSS(); css != "" {
		t.Errorf("no theme: CSS = %q, expected none", css)
	}
	th, err := parseTheme(map[string]any{
		"colors": map[string]any{"primary": "#7c2d2d", "bg": "#f4efe4", "surface": "#fcf9f3"},
		"fonts":  map[string]any{"body": "'Hanken Grotesk', system-ui, sans-serif", "heading": "'Cormorant Garamond', Georgia, serif"},
	})
	if err != nil {
		t.Fatal(err)
	}
	b := &Brand{Theme: th}
	css := b.CSS()
	for _, want := range []string{
		":root:root{", "--primary:#7c2d2d", "--primary-hover:color-mix(in srgb, var(--primary) 82%, black)",
		"--on-primary:#fff", "--bg:#f4efe4", "--surface:#fcf9f3",
		"--font-body:'Hanken Grotesk', system-ui, sans-serif", "--font-heading:'Cormorant Garamond', Georgia, serif",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("CSS = %q, missing %q", css, want)
		}
	}
	if strings.Contains(css, "--text:") || strings.Contains(css, "--accent:") {
		t.Errorf("CSS = %q, declares a token the theme left out", css)
	}
	if got := b.ThemeColor(); got != "#7c2d2d" {
		t.Errorf("ThemeColor = %q, expected the primary when themeColor is left out", got)
	}
	// a light primary gets dark text on it
	if got := onColor("#f5d76e"); got != "#1f2933" {
		t.Errorf("onColor(light) = %q, expected the dark ink", got)
	}
	if got := onColor("#2563eb"); got != "#fff" {
		t.Errorf("onColor(blue) = %q, expected white", got)
	}
}

// TestBrandErrors: a theme or a logo that cannot be what it says fails the
// load, naming the app and the key — a library's too.
func TestBrandErrors(t *testing.T) {
	cases := []struct {
		desk map[string]any
		want string
	}{
		{map[string]any{"theme": map[string]any{"colors": map[string]any{"primay": "#fff"}}}, `unknown field "primay"`},
		{map[string]any{"theme": map[string]any{"colors": map[string]any{"primary": "red;}body{display:none"}}}, "primary"},
		{map[string]any{"theme": map[string]any{"colors": map[string]any{"text": "</style><script>"}}}, "text"},
		{map[string]any{"theme": map[string]any{"colors": map[string]any{"bg": "#12345"}}}, "hex"},
		{map[string]any{"theme": map[string]any{"colors": map[string]any{"bg": "rgb(1,2,3"}}}, "bg"},
		{map[string]any{"theme": map[string]any{"fonts": map[string]any{"body": "x; color: red"}}}, "fonts.body"},
		{map[string]any{"theme": map[string]any{"fonts": map[string]any{"stylesheet": "http://fonts.example/x.css"}}}, "https://"},
		{map[string]any{"theme": map[string]any{"themeColor": "url(x)"}}, "themeColor"},
		{map[string]any{"logo": map[string]any{"image": "../outside.svg"}}, "inside the app"},
		{map[string]any{"logo": map[string]any{"image": "ddcore.app.ts"}}, "must be an image"},
		{map[string]any{"logo": map[string]any{"image": "assets/missing.svg"}}, "does not exist"},
		{map[string]any{"logo": map[string]any{"image": "assets/mark.svg", "favicon": "assets/nope.ico"}}, "favicon"},
		{map[string]any{"logo": map[string]any{"img": "assets/mark.svg"}}, "image"},
	}
	for _, c := range cases {
		st, dirs := brandState(t, "core", "lib", "site")
		st.Snap.Apps["site"].Requires = []string{"lib"}
		writeFile(t, dirs["lib"], "assets/mark.svg", "<svg/>")
		st.Snap.Apps["lib"].Desk = c.desk
		_, err := buildBrand(st.Apps, st.Snap)
		if err == nil || !strings.Contains(err.Error(), `app "lib"`) || !strings.Contains(err.Error(), c.want) {
			t.Errorf("desk %v: error %v, expected one naming app \"lib\" and %q", c.desk, err, c.want)
		}
	}
	// colours a theme may legitimately write
	for _, v := range []string{"#7c2d2d", "#abc", "#7c2d2dcc", "rebeccapurple", "rgb(124 45 45 / .9)", "oklch(0.5 0.1 20)", "color-mix(in srgb, #7c2d2d 80%, white)"} {
		if err := checkColor(v); err != nil {
			t.Errorf("checkColor(%q) = %v, expected a valid colour", v, err)
		}
	}
}
