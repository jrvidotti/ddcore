package api

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// notFound is what the router answers for a path no route claims: an app's
// static site when the first segment is one of their prefixes, the desk
// otherwise. The lookup is per request, against the state of that moment,
// because the router is built once and a reload only swaps the state: a
// prefix added or dropped under `ddcore dev` must take effect without a
// restart.
func (s *Server) notFound(w http.ResponseWriter, r *http.Request) {
	seg, rest, slash := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if st := s.E.Current(); st != nil {
		if site, ok := st.WWW(seg); ok {
			s.www(w, r, site, rest, slash)
			return
		}
	}
	s.deskHandler(w, r)
}

// www serves one file of an app's static site (#68). The files are public:
// what needs protecting is the data, and that is behind the methods the site
// calls. The directory is read on every request, so a rebuild shows at once.
func (s *Server) www(w http.ResponseWriter, r *http.Request, site engine.WWWSite, rest string, slash bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if !slash {
		// The site's relative links resolve against /r/, not /; a SPA built
		// with its base at /r expects to be loaded from there.
		target := "/" + site.Prefix + "/"
		if r.URL.RawQuery != "" {
			target += "?" + r.URL.RawQuery
		}
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
		return
	}
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
	if !site.Frame {
		h.Set("X-Frame-Options", "DENY")
	}
	// r.URL.Path is already decoded, so %2e%2e and ..%2f arrive here as "..".
	// Cleaning would fold "/r/../x" into "/r/x" and serve something; a path
	// that tries to climb is refused instead, whatever it would land on.
	for _, part := range strings.Split(rest, "/") {
		if part == ".." {
			http.NotFound(w, r)
			return
		}
	}
	// os.Root confines every open to the directory, a symlink out of it
	// included, which a string check on the path cannot.
	root, err := os.OpenRoot(site.Dir)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer root.Close()
	name := strings.TrimPrefix(path.Clean("/"+rest), "/")
	f, st, name := openSiteFile(root, name)
	if f == nil && site.Fallback != "" {
		f, st, name = openSiteFile(root, path.Clean(site.Fallback))
	}
	if f == nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	switch {
	case strings.HasSuffix(name, ".html"):
		// the shell names the hashed bundles; a cached one would keep loading
		// a build that no longer exists
		h.Set("Cache-Control", "no-cache")
	case strings.HasPrefix(name, "_app/immutable/") || strings.HasPrefix(name, "immutable/"):
		h.Set("Cache-Control", "public, max-age=31536000, immutable")
	default:
		h.Set("Cache-Control", "public, max-age=300")
	}
	http.ServeContent(w, r, name, st.ModTime(), f)
}

// openSiteFile opens name in root, or its index.html when name is a
// directory. It returns a nil file when there is nothing to serve, and the
// name it actually opened, which decides the content type and the caching.
func openSiteFile(root *os.Root, name string) (*os.File, fs.FileInfo, string) {
	if name == "" {
		name = "."
	}
	for range 2 {
		f, err := root.Open(name)
		if err != nil {
			return nil, nil, ""
		}
		st, err := f.Stat()
		if err != nil {
			f.Close()
			return nil, nil, ""
		}
		if !st.IsDir() {
			return f, st, name
		}
		f.Close()
		name = path.Join(name, "index.html")
	}
	return nil, nil, ""
}
