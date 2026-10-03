package engine

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"sort"

	"github.com/jrvidotti/ddcore/internal/js"
)

// WWWSite is one app's static site, served under /<Prefix>/ (#68): the build
// of a SPA that talks to the app's guest methods, without a server of its own.
type WWWSite struct {
	// Prefix is the first path segment, without slashes.
	Prefix string
	App    string
	// Dir is absolute. It may not exist yet: the site's build can come after
	// the app, and until it does the prefix answers 404.
	Dir string
	// Fallback is the file, relative to Dir, served for a path that names no
	// file — a client-side route. Empty means a missing file is a 404.
	Fallback string
	// Frame lets another page embed the site; without it X-Frame-Options is
	// DENY, since a payment page in someone else's frame is a clickjacking
	// target.
	Frame bool
}

// wwwPrefix is one segment, lower case, because the router compares the
// segment as it arrives: "/R" and "/r" would otherwise be two sites, or a
// site and a desk route, depending on how the visitor typed it.
var wwwPrefix = regexp.MustCompile(`^/[a-z0-9][a-z0-9_-]*$`)

// reservedWWW are the first segments the server or the desk already answer.
// A site there would either never be reached or shadow the desk.
var reservedWWW = map[string]bool{
	"api": true, "app": true, "login": true, "portal": true, "_app": true, "assets": true,
	"files": true, "private": true, "mcp": true, "healthz": true, "readyz": true,
}

// buildWWW validates every app's `www` block and returns the sites by prefix.
// Apps are taken in load order and prefixes sorted, so the same clash is
// reported the same way on every load.
func buildWWW(apps []js.App, snap *Snapshot) (map[string]WWWSite, error) {
	out := map[string]WWWSite{}
	for _, a := range apps {
		am := snap.Apps[a.Name]
		if am == nil || len(am.WWW) == 0 {
			continue
		}
		prefixes := make([]string, 0, len(am.WWW))
		for p := range am.WWW {
			prefixes = append(prefixes, p)
		}
		sort.Strings(prefixes)
		for _, p := range prefixes {
			site, err := parseWWW(a, p, am.WWW[p])
			if err != nil {
				return nil, fmt.Errorf("app %q: www %w", a.Name, err)
			}
			if other, ok := out[site.Prefix]; ok {
				return nil, fmt.Errorf("app %q: www %q is taken by app %q", a.Name, p, other.App)
			}
			out[site.Prefix] = site
		}
	}
	return out, nil
}

func parseWWW(a js.App, prefix string, raw json.RawMessage) (WWWSite, error) {
	if !wwwPrefix.MatchString(prefix) {
		return WWWSite{}, fmt.Errorf("%q must be a single lowercase path segment, like \"/r\"", prefix)
	}
	seg := prefix[1:]
	if reservedWWW[seg] {
		return WWWSite{}, fmt.Errorf("%q is reserved by ddcore", prefix)
	}
	site := WWWSite{Prefix: seg, App: a.Name, Fallback: "index.html"}
	var dir string
	if err := json.Unmarshal(raw, &dir); err != nil {
		// the long form; `fallback` absent keeps the default, null drops it
		var long map[string]json.RawMessage
		if err := json.Unmarshal(raw, &long); err != nil {
			return WWWSite{}, fmt.Errorf("%q: expected a directory or { dir, fallback?, frame? }", prefix)
		}
		if v, ok := long["dir"]; ok {
			json.Unmarshal(v, &dir)
		}
		if v, ok := long["fallback"]; ok {
			var fb *string
			if err := json.Unmarshal(v, &fb); err != nil {
				return WWWSite{}, fmt.Errorf("%q: fallback must be a file name or null", prefix)
			}
			site.Fallback = ""
			if fb != nil {
				site.Fallback = *fb
			}
		}
		if v, ok := long["frame"]; ok {
			if err := json.Unmarshal(v, &site.Frame); err != nil {
				return WWWSite{}, fmt.Errorf("%q: frame must be true or false", prefix)
			}
		}
	}
	if dir == "" {
		return WWWSite{}, fmt.Errorf("%q needs a dir", prefix)
	}
	// IsLocal refuses an absolute path and any that climbs out with "..".
	// The app's own root is refused too: it would publish the app's source,
	// ddcore.app.ts and the services with it.
	if clean := filepath.Clean(dir); !filepath.IsLocal(dir) || clean == "." {
		return WWWSite{}, fmt.Errorf("%q: dir %q must stay inside the app's directory, below its root", prefix, dir)
	} else {
		site.Dir = filepath.Join(a.Dir, clean)
	}
	if site.Fallback != "" && !filepath.IsLocal(site.Fallback) {
		return WWWSite{}, fmt.Errorf("%q: fallback %q must be a file inside dir", prefix, site.Fallback)
	}
	return site, nil
}

// WWW returns the app site served under the first path segment seg, if any.
func (s *State) WWW(seg string) (WWWSite, bool) {
	site, ok := s.www[seg]
	return site, ok
}
