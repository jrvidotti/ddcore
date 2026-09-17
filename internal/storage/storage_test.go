package storage

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
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
	rec := httptest.NewRecorder()
	if err := s.Serve(rec, httptest.NewRequest("GET", "/x", nil), key, Serving{Name: "x.pdf"}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Serve of a missing key: %v", err)
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
