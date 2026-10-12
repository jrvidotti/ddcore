package engine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/js"
)

// DeskTheme is an app's `desk.theme` (#128): the brand colours, the fonts and
// the browser's theme colour of the desk and of the sign-in and portal pages.
// Every key is optional; what is left out keeps the desk's own value.
type DeskTheme struct {
	Colors struct {
		Primary      string `json:"primary"`
		PrimaryHover string `json:"primaryHover"`
		OnPrimary    string `json:"onPrimary"`
		Bg           string `json:"bg"`
		Surface      string `json:"surface"`
		SurfaceAlt   string `json:"surfaceAlt"`
		Border       string `json:"border"`
		Text         string `json:"text"`
		Muted        string `json:"muted"`
		Accent       string `json:"accent"`
		AccentSoft   string `json:"accentSoft"`
	} `json:"colors"`
	Fonts struct {
		Body       string `json:"body"`
		Heading    string `json:"heading"`
		Stylesheet string `json:"stylesheet"`
	} `json:"fonts"`
	ThemeColor string `json:"themeColor"`
}

// BrandFile is one file of an app the site shows as its logo or favicon.
type BrandFile struct {
	App string
	// Dir is the app's directory and Path the file inside it, as declared:
	// the file is opened through an os.Root on Dir, so a symlink cannot lead
	// out of the app.
	Dir  string
	Path string
	// Version changes with the file's content; it goes in the URL so a new
	// logo is not hidden behind a cached one.
	Version string
}

// Brand is the site's look, resolved once per load from the apps' `desk`
// blocks the way SiteDesk resolves `home`: the first app in brandApps order
// that declares `theme` supplies the whole theme, and the first that declares
// `logo` the mark. Nil fields mean the desk's defaults.
type Brand struct {
	Theme    *DeskTheme
	ThemeApp string
	Logo     *BrandFile
	Favicon  *BrandFile
}

// Brand returns the site's brand; never nil.
func (s *State) Brand() *Brand {
	if s.Branding == nil {
		return &Brand{}
	}
	return s.Branding
}

// a CSS colour as a theme may write it: a hex, a name, or a colour function
// like rgb(), oklch() or color-mix(). The character set is what keeps a value
// inside its declaration in the shell's <style>: no ; { } < > quotes or
// backslashes, so it cannot close the rule or the element.
var (
	cssColorChars = regexp.MustCompile(`^[a-zA-Z0-9#%., ()/+-]+$`)
	cssHex        = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{4}|[0-9a-fA-F]{6}|[0-9a-fA-F]{8})$`)
	cssName       = regexp.MustCompile(`^[a-zA-Z]+$`)
	cssFunc       = regexp.MustCompile(`^(rgba?|hsla?|hwb|lab|lch|oklab|oklch|color|color-mix|light-dark)\(.*\)$`)
	// a font-family list: names, quoted or not, separated by commas
	cssFontChars = regexp.MustCompile(`^[a-zA-Z0-9 ,'"_-]+$`)
)

func checkColor(v string) error {
	if len(v) > 120 || !cssColorChars.MatchString(v) {
		return fmt.Errorf("%q is not a CSS colour", v)
	}
	switch {
	case strings.HasPrefix(v, "#"):
		if !cssHex.MatchString(v) {
			return fmt.Errorf("%q is not a hex colour (#rgb, #rrggbb or #rrggbbaa)", v)
		}
	case cssName.MatchString(v):
	case cssFunc.MatchString(v):
		depth := 0
		for _, c := range v {
			if c == '(' {
				depth++
			} else if c == ')' {
				if depth--; depth < 0 {
					break
				}
			}
		}
		if depth != 0 {
			return fmt.Errorf("%q has unbalanced parentheses", v)
		}
	default:
		return fmt.Errorf("%q is not a CSS colour", v)
	}
	return nil
}

// brandImageTypes are the files a logo or favicon may be: images a browser
// shows in an <img> or a tab, and nothing a browser would run.
var brandImageTypes = map[string]string{
	".svg": "image/svg+xml", ".png": "image/png", ".webp": "image/webp", ".jpg": "image/jpeg",
	".jpeg": "image/jpeg", ".gif": "image/gif", ".ico": "image/x-icon",
}

// BrandImageType is the Content-Type a logo or favicon is served with.
func BrandImageType(path string) string {
	return brandImageTypes[strings.ToLower(filepath.Ext(path))]
}

// decodeStrict decodes v (what defineApp declared) into out, refusing keys out
// does not have: a misspelt `primay` would otherwise be a theme that silently
// does nothing.
func decodeStrict(v any, out any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%s", strings.TrimPrefix(err.Error(), "json: "))
	}
	return nil
}

func parseTheme(v any) (*DeskTheme, error) {
	t := &DeskTheme{}
	if err := decodeStrict(v, t); err != nil {
		return nil, fmt.Errorf("desk.theme: %w", err)
	}
	c := t.Colors
	for _, kv := range []struct{ key, v string }{
		{"primary", c.Primary}, {"primaryHover", c.PrimaryHover}, {"onPrimary", c.OnPrimary},
		{"bg", c.Bg}, {"surface", c.Surface}, {"surfaceAlt", c.SurfaceAlt}, {"border", c.Border},
		{"text", c.Text}, {"muted", c.Muted}, {"accent", c.Accent}, {"accentSoft", c.AccentSoft},
		{"themeColor", t.ThemeColor},
	} {
		if kv.v == "" {
			continue
		}
		if err := checkColor(kv.v); err != nil {
			return nil, fmt.Errorf("desk.theme: %s: %w", kv.key, err)
		}
	}
	for _, kv := range []struct{ key, v string }{{"body", t.Fonts.Body}, {"heading", t.Fonts.Heading}} {
		if kv.v != "" && (len(kv.v) > 300 || !cssFontChars.MatchString(kv.v)) {
			return nil, fmt.Errorf("desk.theme: fonts.%s %q is not a font-family list, like \"'Hanken Grotesk', system-ui, sans-serif\"", kv.key, kv.v)
		}
	}
	if s := t.Fonts.Stylesheet; s != "" {
		u, err := url.Parse(s)
		if err != nil || u.Scheme != "https" || u.Host == "" || strings.ContainsAny(s, "\"'<> ") {
			return nil, fmt.Errorf("desk.theme: fonts.stylesheet %q must be an https:// URL", s)
		}
	}
	return t, nil
}

// brandFile checks one declared image path of app a and fingerprints it.
func brandFile(a js.App, key, p string) (*BrandFile, error) {
	if p == "" {
		return nil, fmt.Errorf("desk.logo: %s must be a file of the app, like \"assets/logo.svg\"", key)
	}
	if !filepath.IsLocal(p) {
		return nil, fmt.Errorf("desk.logo: %s %q must stay inside the app's directory", key, p)
	}
	if BrandImageType(p) == "" {
		return nil, fmt.Errorf("desk.logo: %s %q must be an image (.svg, .png, .webp, .jpg, .gif or .ico)", key, p)
	}
	root, err := os.OpenRoot(a.Dir)
	if err != nil {
		return nil, fmt.Errorf("desk.logo: %s %q: %w", key, p, err)
	}
	defer root.Close()
	f, err := root.Open(filepath.Clean(p))
	if err != nil {
		return nil, fmt.Errorf("desk.logo: %s %q does not exist in the app", key, p)
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, fmt.Errorf("desk.logo: %s %q: %w", key, p, err)
	}
	return &BrandFile{App: a.Name, Dir: a.Dir, Path: filepath.Clean(p), Version: hex.EncodeToString(h.Sum(nil))[:12]}, nil
}

// parseLogo reads an object-valued `desk.logo`: { image, favicon? }. The
// string form, a letter or an emoji, is SiteDesk's and returns nils here.
func parseLogo(a js.App, v any) (logo, favicon *BrandFile, err error) {
	if _, ok := v.(string); ok || v == nil {
		return nil, nil, nil
	}
	var d struct {
		Image   string `json:"image"`
		Favicon string `json:"favicon"`
	}
	if err := decodeStrict(v, &d); err != nil {
		return nil, nil, fmt.Errorf("desk.logo: expected one letter or emoji, or { image, favicon? }: %w", err)
	}
	if logo, err = brandFile(a, "image", d.Image); err != nil {
		return nil, nil, err
	}
	if d.Favicon != "" {
		if favicon, err = brandFile(a, "favicon", d.Favicon); err != nil {
			return nil, nil, err
		}
	}
	return logo, favicon, nil
}

// buildBrand validates every app's `desk.theme` and object `desk.logo` — a
// library's too, so its mistakes show where it is written rather than on the
// first site that lets it decide — and resolves the site's.
func buildBrand(apps []js.App, snap *Snapshot) (*Brand, error) {
	dirs := map[string]js.App{}
	for _, a := range apps {
		dirs[a.Name] = a
	}
	b := &Brand{}
	logoDecided := false
	for _, m := range (&State{Apps: apps, Snap: snap}).brandApps() {
		a := dirs[m.Name]
		if v, ok := m.Desk["theme"]; ok && v != nil {
			t, err := parseTheme(v)
			if err != nil {
				return nil, fmt.Errorf("app %q: %w", m.Name, err)
			}
			if b.Theme == nil {
				b.Theme, b.ThemeApp = t, m.Name
			}
		}
		if v, ok := m.Desk["logo"]; ok && v != nil && v != "" {
			logo, favicon, err := parseLogo(a, v)
			if err != nil {
				return nil, fmt.Errorf("app %q: %w", m.Name, err)
			}
			// a letter decides as much as an image: a library's image does not
			// replace the site's own letter
			if !logoDecided {
				b.Logo, b.Favicon, logoDecided = logo, favicon, true
			}
		}
	}
	return b, nil
}

// CSS is the rule that redefines the desk's tokens for the theme, or "" for a
// site without one. It is written for `:root:root` so it outranks the desk's
// own `:root` whichever stylesheet comes last. What the theme leaves out and
// the desk derives from another token on its own (hover, selection, focus,
// surfaceAlt, accent) follows from the colours given; primaryHover and
// onPrimary are derived here, since the desk's defaults for them are literals
// tied to its blue.
func (b *Brand) CSS() string {
	t := b.Theme
	if t == nil {
		return ""
	}
	var decl []string
	add := func(name, v string) {
		if v != "" {
			decl = append(decl, name+":"+v)
		}
	}
	c := t.Colors
	add("--primary", c.Primary)
	if c.PrimaryHover != "" {
		add("--primary-hover", c.PrimaryHover)
	} else if c.Primary != "" {
		add("--primary-hover", "color-mix(in srgb, var(--primary) 82%, black)")
	}
	if c.OnPrimary != "" || c.Primary != "" {
		add("--on-primary", b.OnPrimary())
	}
	add("--bg", c.Bg)
	add("--surface", c.Surface)
	add("--surface-alt", c.SurfaceAlt)
	add("--border", c.Border)
	add("--text", c.Text)
	add("--muted", c.Muted)
	add("--accent", c.Accent)
	add("--accent-soft", c.AccentSoft)
	add("--font-body", t.Fonts.Body)
	add("--font-heading", t.Fonts.Heading)
	if len(decl) == 0 {
		return ""
	}
	return ":root:root{" + strings.Join(decl, ";") + "}"
}

// OnPrimary is what is written on the primary colour: the theme's, else
// white or near-black, whichever reads on the theme's primary.
func (b *Brand) OnPrimary() string {
	if b.Theme == nil {
		return "#fff"
	}
	if c := b.Theme.Colors.OnPrimary; c != "" {
		return c
	}
	return onColor(b.Theme.Colors.Primary)
}

// ThemeColor is the browser's theme colour: the theme's own, else its
// primary; "" leaves the browser's.
func (b *Brand) ThemeColor() string {
	if b.Theme == nil {
		return ""
	}
	if b.Theme.ThemeColor != "" {
		return b.Theme.ThemeColor
	}
	return b.Theme.Colors.Primary
}

// Stylesheet is the fonts' stylesheet URL, or "".
func (b *Brand) Stylesheet() string {
	if b.Theme == nil {
		return ""
	}
	return b.Theme.Fonts.Stylesheet
}

// onColor picks white or near-black, whichever reads on a hex background; a
// colour function cannot be measured here, and keeps the desk's white.
func onColor(bg string) string {
	if !cssHex.MatchString(bg) {
		return "#fff"
	}
	h := bg[1:]
	if len(h) == 3 || len(h) == 4 {
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	ch := func(i int) float64 {
		n, _ := strconv.ParseUint(h[i:i+2], 16, 8)
		v := float64(n) / 255
		if v <= 0.03928 {
			return v / 12.92
		}
		return math.Pow((v+0.055)/1.055, 2.4)
	}
	l := 0.2126*ch(0) + 0.7152*ch(2) + 0.0722*ch(4)
	// contrast against white vs against #1f2933 (L≈0.022)
	if (1.05)/(l+0.05) >= (l+0.05)/(0.022+0.05) {
		return "#fff"
	}
	return "#1f2933"
}
