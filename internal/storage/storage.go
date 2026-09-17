// Package storage keeps the bytes of uploaded files. A File document's
// file_url is the only name the rest of the system uses; KeyFromURL turns it
// into the key a Store understands, so the database never records which
// backend holds the bytes and a site can move between backends by copying.
package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
)

// ErrNotFound is returned when no object exists under a key.
var ErrNotFound = errors.New("storage: object not found")

// Info describes a stored object.
type Info struct {
	Size    int64
	ModTime time.Time
}

// Serving is how a download is presented to the browser.
type Serving struct {
	Name   string // file name the browser is told
	Inline bool   // display in place rather than download
}

// Store is a place to keep file bytes.
type Store interface {
	Put(ctx context.Context, key string, r io.Reader, size int64, contentType string) error
	Open(ctx context.Context, key string) (io.ReadCloser, Info, error)
	// Delete removes an object; a key that holds nothing is not an error.
	Delete(ctx context.Context, key string) error
	// Serve answers a download request for an object whose permission check
	// the caller has already made.
	Serve(w http.ResponseWriter, r *http.Request, key string, s Serving) error
	// List calls fn for every object whose key starts with prefix, in no
	// particular order. It is how a backup reaches bytes no File row names.
	List(ctx context.Context, prefix string, fn func(key string, info Info) error) error
	// Backend names the store for logs and `ddcore doctor`.
	Backend() string
}

// New builds the store the configuration asks for. dataDir is where the
// local backend keeps files.
func New(ctx context.Context, cfg config.Storage, dataDir string) (Store, error) {
	switch cfg.Backend {
	case "", config.StorageLocal:
		if dataDir == "" {
			dataDir = "data"
		}
		return NewLocal(path.Join(dataDir, "files")), nil
	case config.StorageS3:
		return NewS3(ctx, cfg.S3)
	}
	return nil, fmt.Errorf("storage: unknown backend %q", cfg.Backend)
}

// KeyFromURL maps a File's url to its storage key: "/files/x.png" is
// "public/x.png" and "/private/files/x.pdf" is "private/x.pdf". Anything that
// is not exactly one plain name under one of those prefixes is refused, so a
// request path can never reach outside them.
func KeyFromURL(fileURL string) (string, bool) {
	var sub, name string
	switch {
	case strings.HasPrefix(fileURL, "/private/files/"):
		sub, name = "private", strings.TrimPrefix(fileURL, "/private/files/")
	case strings.HasPrefix(fileURL, "/files/"):
		sub, name = "public", strings.TrimPrefix(fileURL, "/files/")
	default:
		return "", false
	}
	if name == "" || strings.HasPrefix(name, ".") || strings.ContainsAny(name, "/\\\x00") {
		return "", false
	}
	return sub + "/" + name, true
}

// ReadAll reads a whole object.
func ReadAll(ctx context.Context, s Store, key string) ([]byte, error) {
	rc, _, err := s.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// servedType is the content type a download is announced with: a displayable
// type for inline files, and bytes for everything else, whatever the uploader
// claimed.
func servedType(s Serving) string {
	if s.Inline {
		if t := mime.TypeByExtension(strings.ToLower(path.Ext(s.Name))); t != "" {
			return t
		}
	}
	return "application/octet-stream"
}

func disposition(s Serving) string {
	kind := "attachment"
	if s.Inline {
		kind = "inline"
	}
	return mime.FormatMediaType(kind, map[string]string{"filename": s.Name})
}
