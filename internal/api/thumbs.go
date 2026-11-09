package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/jrvidotti/ddcore/internal/storage"
	"github.com/jrvidotti/ddcore/internal/thumb"
)

// thumbMaker makes the thumbnails of public images on their first request
// (#154). Concurrent first requests for one variant share a single decode,
// and only a couple of decodes run at once: a large photo takes hundreds of
// megabytes while it is in memory.
type thumbMaker struct {
	flight singleflight.Group
	slots  chan struct{}
}

func newThumbMaker() *thumbMaker { return &thumbMaker{slots: make(chan struct{}, 2)} }

// thumbMaxSource is the largest original resized; a bigger one is served as is.
const thumbMaxSource = 50 << 20

// thumbExts are the images a thumbnail is made of: the ones served inline.
var thumbExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true}

// thumbWidth is the w of ?w=, when it is one of storage.ThumbWidths.
func thumbWidth(q string) (int, bool) {
	for _, w := range storage.ThumbWidths {
		if strconv.Itoa(w) == q {
			return w, true
		}
	}
	return 0, false
}

func thumbWidthList() string {
	parts := make([]string, len(storage.ThumbWidths))
	for i, w := range storage.ThumbWidths {
		parts[i] = strconv.Itoa(w)
	}
	return strings.Join(parts, ", ")
}

type madeThumb struct {
	data []byte
	ext  string // "" when the original is to be served instead
}

// serveThumb answers /files/<name>?w=<width> from the stored thumbnail, or
// makes it. It reports false when the image cannot be resized — too large, or
// not decodable — and the original is to be served: that outcome is the same
// on every request, so the original under this URL may be cached too.
func (s *Server) serveThumb(w http.ResponseWriter, r *http.Request, key string, width int) (bool, error) {
	store := s.E.Storage()
	name := path.Base(key)
	stem := strings.TrimSuffix(name, path.Ext(name))
	// a JPEG is opaque, so its thumbnail is a JPEG; any other may be either
	exts := []string{".jpg"}
	if e := strings.ToLower(path.Ext(name)); e != ".jpg" && e != ".jpeg" {
		exts = append(exts, ".png")
	}
	for _, ext := range exts {
		err := store.Serve(w, r, storage.ThumbKey(key, width, ext), storage.Serving{Name: stem + ext, Inline: true, Public: true})
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, storage.ErrNotFound) {
			return false, err
		}
	}
	v, err, _ := s.thumbs.flight.Do(fmt.Sprintf("%d/%s", width, key), func() (any, error) {
		return s.makeThumb(key, width)
	})
	if err != nil {
		return false, err
	}
	t := v.(madeThumb)
	if t.ext == "" {
		return false, nil
	}
	storage.ServeReader(w, r, bytes.NewReader(t.data), storage.Info{Size: int64(len(t.data)), ModTime: time.Now()},
		storage.Serving{Name: stem + t.ext, Inline: true, Public: true})
	return true, nil
}

// makeThumb resizes the original and stores the result. It runs on its own
// context: the requests sharing it must not lose it to the first one going.
func (s *Server) makeThumb(key string, width int) (madeThumb, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	select {
	case s.thumbs.slots <- struct{}{}:
		defer func() { <-s.thumbs.slots }()
	case <-ctx.Done():
		return madeThumb{}, ctx.Err()
	}
	store := s.E.Storage()
	rc, info, err := store.Open(ctx, key)
	if err != nil {
		return madeThumb{}, err
	}
	defer rc.Close()
	if info.Size > thumbMaxSource {
		s.E.Log.Info("thumbnail: original too large to resize, serving it as is", "key", key, "size", info.Size)
		return madeThumb{}, nil
	}
	src, err := io.ReadAll(io.LimitReader(rc, thumbMaxSource+1))
	if err != nil {
		return madeThumb{}, err
	}
	if len(src) > thumbMaxSource {
		return madeThumb{}, nil
	}
	out, ext, err := thumb.Make(src, path.Ext(key), width)
	if err != nil {
		s.E.Log.Warn("thumbnail: cannot resize, serving the original", "key", key, "err", err)
		return madeThumb{}, nil
	}
	ct := "image/jpeg"
	if ext == ".png" {
		ct = "image/png"
	}
	if err := store.Put(ctx, storage.ThumbKey(key, width, ext), bytes.NewReader(out), int64(len(out)), ct); err != nil {
		// served all the same; the next request makes it again
		s.E.Log.Warn("thumbnail: not stored", "key", key, "width", width, "err", err)
	}
	return madeThumb{data: out, ext: ext}, nil
}
