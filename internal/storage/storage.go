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

// ErrPresignUnsupported is returned by a backend that cannot hand out a URL
// that works without a session.
var ErrPresignUnsupported = errors.New("storage: this backend cannot presign a URL")

// Info describes a stored object.
type Info struct {
	Size    int64
	ModTime time.Time
	ETag    string // unquoted; empty when the backend keeps none
}

// Serving is how a download is presented to the browser.
type Serving struct {
	Name   string // file name the browser is told
	Inline bool   // display in place rather than download
	// Public is a file served to anyone without a check. Its name is random
	// and the bytes under it never change, so it may be cached for good.
	Public bool
}

// immutable is how long a public file may be kept: a year, the most a cache
// honours, without asking again.
const immutable = "public, max-age=31536000, immutable"

// serveContent streams an object the caller has already opened, so the headers
// below are only ever sent for bytes that exist: a 404 must not be cached for
// a year. http.ServeContent answers If-None-Match / If-Modified-Since with 304
// before reading anything, and serves Range requests.
func serveContent(w http.ResponseWriter, r *http.Request, rs io.ReadSeeker, info Info, s Serving) {
	h := w.Header()
	h.Set("Content-Disposition", disposition(s))
	h.Set("Content-Type", servedType(s))
	if info.ETag != "" {
		h.Set("ETag", `"`+info.ETag+`"`)
	}
	if s.Public {
		h.Set("Cache-Control", immutable)
	}
	http.ServeContent(w, r, s.Name, info.ModTime, rs)
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
	// Presign returns a URL that answers anyone holding it with the object
	// until ttl passes: how a third party with no session fetches a private
	// file. The caller has already made the permission check.
	Presign(ctx context.Context, key string, ttl time.Duration, s Serving) (string, error)
	// List calls fn for every object whose key starts with prefix, in no
	// particular order. It is how a backup reaches bytes no File row names.
	List(ctx context.Context, prefix string, fn func(key string, info Info) error) error
	// DeletePrefix removes every object under prefix, which ends in "/": how
	// a file's thumbnails go with it. A prefix that holds nothing is not an
	// error.
	DeletePrefix(ctx context.Context, prefix string) error
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

// ThumbWidths are the only sizes a public image's thumbnail comes in, so the
// variants of one file cannot be multiplied at will (#154). The helpers in
// internal/js/prelude.js and desk/src/lib/thumbs.ts repeat this list.
var ThumbWidths = []int{64, 128, 256, 512}

// ThumbKey is where the w-pixel thumbnail of a public key is kept:
// thumbs/<name>/<w>.<ext>. It is outside public/ and private/, so no URL
// reaches it by KeyFromURL and a backup, which copies only those two, leaves
// it out: a thumbnail is made again on the first request after a restore.
func ThumbKey(key string, w int, ext string) string {
	return fmt.Sprintf("%s%d%s", ThumbPrefix(key), w, ext)
}

// ThumbPrefix holds every thumbnail of a public key.
func ThumbPrefix(key string) string { return "thumbs/" + path.Base(key) + "/" }

// ServeReader streams bytes the caller already holds — a thumbnail just made —
// with the headers Serve would send for them.
func ServeReader(w http.ResponseWriter, r *http.Request, rs io.ReadSeeker, info Info, s Serving) {
	serveContent(w, r, rs, info, s)
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

// AudioTypes are the recordings served in place, so an <audio> element plays
// them — a feedback's voice note above all. Go's own table knows none of them,
// and the system's may not exist in a container, so they are named here.
var AudioTypes = map[string]string{
	".webm": "audio/webm", ".ogg": "audio/ogg", ".oga": "audio/ogg", ".opus": "audio/ogg",
	".m4a": "audio/mp4", ".mp3": "audio/mpeg", ".wav": "audio/wav",
}

// servedType is the content type a download is announced with: a displayable
// type for inline files, and bytes for everything else, whatever the uploader
// claimed.
func servedType(s Serving) string {
	if s.Inline {
		if t, ok := AudioTypes[strings.ToLower(path.Ext(s.Name))]; ok {
			return t
		}
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
