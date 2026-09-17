package storage

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// Local keeps files under a directory on this machine, in the layout uploads
// have always used: <root>/public and <root>/private.
type Local struct{ Root string }

func NewLocal(root string) *Local { return &Local{Root: root} }

func (l *Local) Backend() string { return "local" }

func (l *Local) path(key string) string { return filepath.Join(l.Root, filepath.FromSlash(key)) }

func (l *Local) Put(_ context.Context, key string, r io.Reader, _ int64, _ string) error {
	p := l.path(key)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	out, err := os.Create(p)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, r); err != nil {
		out.Close()
		os.Remove(p)
		return err
	}
	return out.Close()
}

func (l *Local) Open(_ context.Context, key string) (io.ReadCloser, Info, error) {
	f, err := os.Open(l.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, Info{}, ErrNotFound
	}
	if err != nil {
		return nil, Info{}, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, Info{}, err
	}
	if st.IsDir() {
		f.Close()
		return nil, Info{}, ErrNotFound
	}
	return f, Info{Size: st.Size(), ModTime: st.ModTime()}, nil
}

func (l *Local) Delete(_ context.Context, key string) error {
	err := os.Remove(l.path(key))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// Serve streams the file itself. It never lists a directory, which the
// http.FileServer it replaces did for a bare /files/.
func (l *Local) Serve(w http.ResponseWriter, r *http.Request, key string, s Serving) error {
	rc, info, err := l.Open(r.Context(), key)
	if err != nil {
		return err
	}
	defer rc.Close()
	w.Header().Set("Content-Disposition", disposition(s))
	w.Header().Set("Content-Type", servedType(s))
	http.ServeContent(w, r, s.Name, info.ModTime, rc.(*os.File))
	return nil
}

// List walks the directory. A prefix that names no directory yet — a site that
// has never had a private upload — lists nothing rather than failing.
func (l *Local) List(ctx context.Context, prefix string, fn func(key string, info Info) error) error {
	root := filepath.Clean(l.Root)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if !strings.HasPrefix(key, prefix) {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		return fn(key, Info{Size: st.Size(), ModTime: st.ModTime()})
	})
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
