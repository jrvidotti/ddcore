package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
)

func TestKeyFromURL(t *testing.T) {
	for _, tc := range []struct {
		url, key string
		ok       bool
	}{
		{"/files/a.png", "public/a.png", true},
		{"/private/files/b.pdf", "private/b.pdf", true},
		{"/files/", "", false},
		{"/files/..", "", false},
		{"/files/.env", "", false},
		{"/files/x/../../etc", "", false},
		{"/private/files/a\\b", "", false},
		{"/other/a.png", "", false},
	} {
		key, ok := KeyFromURL(tc.url)
		if key != tc.key || ok != tc.ok {
			t.Errorf("KeyFromURL(%q) = %q, %v; want %q, %v", tc.url, key, ok, tc.key, tc.ok)
		}
	}
}

// exercise is the contract every backend has to meet.
func exercise(t *testing.T, s Store) {
	ctx := context.Background()
	key := "private/test-" + time.Now().Format("150405.000000") + ".pdf"
	body := []byte("%PDF bytes")
	if err := s.Put(ctx, key, bytes.NewReader(body), int64(len(body)), "text/html"); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll(ctx, s, key)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("ReadAll: %q %v", got, err)
	}
	listed := map[string]int64{}
	if err := s.List(ctx, "private/", func(k string, info Info) error {
		listed[k] = info.Size
		return nil
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if listed[key] != int64(len(body)) {
		t.Fatalf("List did not return %s with its size: %v", key, listed)
	}
	if err := s.List(ctx, "public/", func(k string, _ Info) error {
		if k == key {
			t.Errorf("List(public/) returned %s", k)
		}
		return nil
	}); err != nil {
		t.Fatalf("List: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Open(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open after Delete: %v", err)
	}
	if err := s.Delete(ctx, key); err != nil {
		t.Fatalf("Delete of a missing key: %v", err)
	}
	for _, public := range []bool{false, true} {
		rec := httptest.NewRecorder()
		if err := s.Serve(rec, httptest.NewRequest("GET", "/x", nil), key, Serving{Name: "x.pdf", Public: public}); !errors.Is(err, ErrNotFound) {
			t.Fatalf("Serve of a missing key (public %v): %v", public, err)
		}
		// a 404 must not leave a year-long Cache-Control behind
		if cc := rec.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("Serve of a missing key set Cache-Control %q", cc)
		}
	}

	// DeletePrefix takes everything under one directory and nothing beside it
	stamp := time.Now().Format("150405.000000")
	for _, k := range []string{"thumbs/a" + stamp + "/64.jpg", "thumbs/a" + stamp + "/128.png", "thumbs/b" + stamp + "/64.jpg"} {
		if err := s.Put(ctx, k, strings.NewReader("x"), 1, "image/jpeg"); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.DeletePrefix(ctx, "thumbs/a"+stamp+"/"); err != nil {
		t.Fatalf("DeletePrefix: %v", err)
	}
	for k, want := range map[string]bool{"thumbs/a" + stamp + "/64.jpg": false, "thumbs/a" + stamp + "/128.png": false, "thumbs/b" + stamp + "/64.jpg": true} {
		rc, _, err := s.Open(ctx, k)
		if err == nil {
			rc.Close()
		}
		if got := err == nil; got != want {
			t.Errorf("after DeletePrefix, %s exists = %v (%v)", k, got, err)
		}
	}
	if err := s.DeletePrefix(ctx, "thumbs/b"+stamp+"/"); err != nil {
		t.Fatal(err)
	}
	if err := s.DeletePrefix(ctx, "thumbs/none"+stamp+"/"); err != nil {
		t.Errorf("DeletePrefix of an empty prefix: %v", err)
	}
	if err := s.DeletePrefix(ctx, "thumbs"); err == nil {
		t.Error("DeletePrefix took a prefix that is no directory")
	}
}

// servePublic checks what a public file is served with on every backend: the
// bytes, the served type, a year-long immutable Cache-Control, and answers to
// a conditional or a range request.
func servePublic(t *testing.T, s Store) {
	ctx := context.Background()
	key := "public/pub-" + time.Now().Format("150405.000000") + ".png"
	if err := s.Put(ctx, key, strings.NewReader("png-bytes"), 9, "text/html"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Delete(context.Background(), key) })
	sv := Serving{Name: path.Base(key), Inline: true, Public: true}
	get := func(h map[string]string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest("GET", "/files/"+path.Base(key), nil)
		for k, v := range h {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		if err := s.Serve(rec, req, key, sv); err != nil {
			t.Fatal(err)
		}
		return rec
	}
	rec := get(nil)
	if rec.Code != 200 || rec.Body.String() != "png-bytes" {
		t.Fatalf("public Serve: %d %q (Location %q)", rec.Code, rec.Body.String(), rec.Header().Get("Location"))
	}
	h := rec.Header()
	if h.Get("Cache-Control") != "public, max-age=31536000, immutable" || h.Get("Content-Type") != "image/png" ||
		!strings.HasPrefix(h.Get("Content-Disposition"), "inline") || h.Get("Last-Modified") == "" {
		t.Errorf("public Serve headers: %v", h)
	}
	if etag := h.Get("ETag"); etag != "" {
		if !strings.HasPrefix(etag, `"`) {
			t.Errorf("ETag not quoted: %q", etag)
		}
		if r := get(map[string]string{"If-None-Match": etag}); r.Code != http.StatusNotModified {
			t.Errorf("If-None-Match: %d", r.Code)
		}
	}
	if r := get(map[string]string{"If-Modified-Since": h.Get("Last-Modified")}); r.Code != http.StatusNotModified {
		t.Errorf("If-Modified-Since: %d", r.Code)
	} else if r.Header().Get("Cache-Control") == "" {
		t.Error("a 304 dropped the Cache-Control")
	}
	if r := get(map[string]string{"Range": "bytes=1-2"}); r.Code != http.StatusPartialContent || r.Body.String() != "ng" {
		t.Errorf("Range: %d %q", r.Code, r.Body.String())
	}
}

func TestLocal(t *testing.T) {
	root := t.TempDir()
	s := NewLocal(root)
	exercise(t, s)

	ctx := context.Background()
	if err := s.Put(ctx, "public/a.txt", strings.NewReader("hello"), 5, ""); err != nil {
		t.Fatal(err)
	}
	// the layout uploads have always had, so an existing data dir keeps working
	if b, err := os.ReadFile(filepath.Join(root, "public", "a.txt")); err != nil || string(b) != "hello" {
		t.Fatalf("layout: %q %v", b, err)
	}
	rec := httptest.NewRecorder()
	if err := s.Serve(rec, httptest.NewRequest("GET", "/files/a.txt", nil), "public/a.txt", Serving{Name: "a.txt"}); err != nil {
		t.Fatal(err)
	}
	if rec.Code != 200 || rec.Body.String() != "hello" {
		t.Fatalf("Serve: %d %q", rec.Code, rec.Body.String())
	}
	if ct, cd := rec.Header().Get("Content-Type"), rec.Header().Get("Content-Disposition"); ct != "application/octet-stream" || cd != "attachment; filename=a.txt" {
		t.Errorf("headers: %q %q", ct, cd)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "" {
		t.Errorf("a file not served as public got Cache-Control %q", cc)
	}
	servePublic(t, s)
	if _, _, err := s.Open(ctx, "public"); !errors.Is(err, ErrNotFound) {
		t.Errorf("a directory must not open: %v", err)
	}
}

// TestS3 runs against a real S3-compatible server, e.g. the MinIO service in
// docker-compose.yml:
//
//	DDCORE_TEST_S3_ENDPOINT=localhost:9000 go test ./internal/storage/
func TestS3(t *testing.T) {
	endpoint := os.Getenv("DDCORE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("DDCORE_TEST_S3_ENDPOINT not set")
	}
	envOr := func(k, def string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return def
	}
	cfg := config.S3{
		Endpoint: endpoint, Bucket: envOr("DDCORE_TEST_S3_BUCKET", "ddcore"), Region: "us-east-1",
		AccessKey: envOr("DDCORE_TEST_S3_ACCESS_KEY", "ddcore"), SecretKey: envOr("DDCORE_TEST_S3_SECRET_KEY", "ddcore-secret"),
		Prefix: "test", PathStyle: true, PresignTTL: time.Minute,
	}
	s, err := NewS3(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	exercise(t, s)
	servePublic(t, s)

	ctx := context.Background()
	if err := s.Put(ctx, "private/p.pdf", strings.NewReader("pdf"), 3, "text/html"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Delete(context.Background(), "private/p.pdf") })
	rec := httptest.NewRecorder()
	if err := s.Serve(rec, httptest.NewRequest("GET", "/private/files/p.pdf", nil), "private/p.pdf", Serving{Name: "p.pdf", Inline: true}); err != nil {
		t.Fatal(err)
	}
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusFound || !strings.Contains(loc, "X-Amz-Signature=") || !strings.Contains(loc, "/test/private/p.pdf") {
		t.Fatalf("Serve: %d %s", rec.Code, loc)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Errorf("private redirect Cache-Control: %q", cc)
	}
	res, err := http.Get(loc)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	// the uploader said text/html; the bucket must not repeat it
	if res.StatusCode != 200 || res.Header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(res.Header.Get("Content-Disposition"), "inline") {
		t.Errorf("presigned GET: %d %q %q", res.StatusCode, res.Header.Get("Content-Type"), res.Header.Get("Content-Disposition"))
	}
}
