package engine

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/config"
)

// The test runners build their engine with WithLocalFiles: the bytes a test
// saves land in the directory handed over, never in the site's bucket (#174).
func TestWithLocalFilesKeepsBytesOffTheBucket(t *testing.T) {
	dir := t.TempDir()
	site := Config{DataDir: "/srv/data", Storage: config.Storage{Backend: config.StorageS3,
		S3: config.S3{Endpoint: "bucket.invalid", Bucket: "site", AccessKey: "k", SecretKey: "s", Fallback: true, FallbackPrefix: "prod"}}}
	e, err := New(context.Background(), site.WithLocalFiles(dir))
	if err != nil {
		t.Fatal(err)
	}
	if b := e.Storage().Backend(); b != config.StorageLocal {
		t.Fatalf("backend = %q, expected local", b)
	}
	if err := e.Storage().Put(context.Background(), "private/a.txt", strings.NewReader("x"), 1, "text/plain"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "files", "private", "a.txt")); err != nil || string(b) != "x" {
		t.Fatalf("the file is not in the directory: %q, %v", b, err)
	}
	if site.Storage.Backend != config.StorageS3 || site.DataDir != "/srv/data" {
		t.Fatal("WithLocalFiles changed the configuration it was called on")
	}
}
