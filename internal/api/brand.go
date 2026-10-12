package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html"
	"net/http"
	"os"
	"strings"
	"unicode"

	"github.com/go-chi/chi/v5"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// The site's brand (#128): the theme and the logo an app declares in `desk`,
// written into the shell so the first paint already wears them — the sign-in
// page included, which a desk include never reached — and repeated in boot
// for the desk to apply again after a reload under `ddcore dev`.

// brandBoot is `site.brand` in /api/boot; each field only when there is one.
func brandBoot(st *engine.State, letter string) map[string]any {
	b := st.Brand()
	out := map[string]any{"favicon": faviconURL(b, letter)}
	if css := b.CSS(); css != "" {
		out["css"] = css
	}
	if s := b.Stylesheet(); s != "" {
		out["stylesheet"] = s
	}
	if c := b.ThemeColor(); c != "" {
		out["themeColor"] = c
	}
	if b.Logo != nil {
		out["logoImage"] = "/assets/site/logo?v=" + b.Logo.Version
	}
	return out
}

// siteLetter is the mark's letter for the generated favicon: the declared
// one, else the initial of the site's name as the reader sees it — what the
// desk's siteLogo() draws.
func siteLetter(st *engine.State, name string) string {
	if l := st.SiteDesk("logo"); l != "" {
		return l
	}
	for _, r := range strings.TrimSpace(name) {
		return string(unicode.ToUpper(r))
	}
	return "d"
}

func faviconURL(b *engine.Brand, letter string) string {
	switch {
	case b.Favicon != nil:
		return "/assets/site/favicon?v=" + b.Favicon.Version
	case b.Logo != nil:
		return "/assets/site/favicon?v=" + b.Logo.Version
	}
	return "/assets/site/favicon?v=" + generatedVersion(b, letter)
}

// generatedFavicon is the favicon of a site without an image: the letter
// square of the sidebar, in the theme's primary colour.
func generatedFavicon(b *engine.Brand, letter string) []byte {
	bg, fg := "#2563eb", "#fff"
	if b.Theme != nil && b.Theme.Colors.Primary != "" {
		bg = b.Theme.Colors.Primary
		fg = b.OnPrimary()
	}
	return []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">` +
		`<rect width="64" height="64" rx="14" fill="` + html.EscapeString(bg) + `"/>` +
		`<text x="32" y="33" text-anchor="middle" dominant-baseline="central" font-family="system-ui, sans-serif" font-size="36" font-weight="700" fill="` + html.EscapeString(fg) + `">` +
		html.EscapeString(letter) + `</text></svg>`)
}

func generatedVersion(b *engine.Brand, letter string) string {
	h := sha256.Sum256(generatedFavicon(b, letter))
	return hex.EncodeToString(h[:])[:12]
}

// siteAsset serves /assets/site/logo and /assets/site/favicon: the files the
// site's app declared in `desk.logo`, public like the sign-in page that shows
// them. Only the declared file is reachable — the route names a role, not a
// path — and it is opened through an os.Root, so a symlink cannot lead out of
// the app.
func (s *Server) siteAsset(w http.ResponseWriter, r *http.Request) {
	st := s.E.Current()
	b := st.Brand()
	var f *engine.BrandFile
	switch chi.URLParam(r, "name") {
	case "logo":
		f = b.Logo
	case "favicon":
		f = b.Favicon
		if f == nil {
			f = b.Logo
		}
		if f == nil {
			letter := siteLetter(st, st.I18n.T(s.langFor(r), st.SiteTitle()))
			s.brandHeaders(w, r, generatedVersion(b, letter))
			w.Header().Set("Content-Type", "image/svg+xml")
			w.Write(generatedFavicon(b, letter))
			return
		}
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	root, err := os.OpenRoot(f.Dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	file, err := root.Open(f.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}
	s.brandHeaders(w, r, f.Version)
	w.Header().Set("Content-Type", engine.BrandImageType(f.Path))
	http.ServeContent(w, r, f.Path, info.ModTime(), file)
}

// brandHeaders: a URL carrying the current version is the file for good; any
// other is revalidated, since it names whatever the site declares now.
func (s *Server) brandHeaders(w http.ResponseWriter, r *http.Request, version string) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	// an SVG opened on its own is a document: it may style itself, nothing more
	h.Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
	if r.URL.Query().Get("v") == version && !s.E.Cfg.Dev {
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		h.Set("Cache-Control", "no-cache")
	}
}

// withBrand writes the site's name and look into the shell: the <title> the
// tab shows before the desk boots, and before </head> the favicon, the
// browser's theme colour, the fonts' stylesheet and the theme's tokens — last,
// so they follow the desk's own stylesheet.
func withBrand(page []byte, st *engine.State, name string) []byte {
	b := st.Brand()
	page = bytes.Replace(page, []byte("<title>ddcore</title>"), []byte("<title>"+html.EscapeString(name)+"</title>"), 1)
	var head strings.Builder
	head.WriteString(`<link rel="icon" href="` + html.EscapeString(faviconURL(b, siteLetter(st, name))) + `">`)
	if c := b.ThemeColor(); c != "" {
		head.WriteString(`<meta name="theme-color" content="` + html.EscapeString(c) + `">`)
	}
	if u := b.Stylesheet(); u != "" {
		head.WriteString(`<link id="ddcore-fonts" rel="stylesheet" href="` + html.EscapeString(u) + `">`)
	}
	if css := b.CSS(); css != "" {
		// the values were checked to hold no < or quote when the app loaded
		head.WriteString(`<style id="ddcore-brand">` + css + `</style>`)
	}
	head.WriteString("</head>")
	return bytes.Replace(page, []byte("</head>"), []byte(head.String()), 1)
}
