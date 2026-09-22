package api

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// testStorage is the local default, or the bucket DDCORE_TEST_S3_ENDPOINT
// points at (the MinIO profile in docker-compose.yml), so every API test can
// be run once against each backend.
func testStorage(t *testing.T) config.Storage {
	endpoint := os.Getenv("DDCORE_TEST_S3_ENDPOINT")
	if endpoint == "" {
		return config.Storage{}
	}
	return config.Storage{Backend: config.StorageS3, S3: config.S3{
		Endpoint: endpoint, Region: "us-east-1", Bucket: "ddcore", AccessKey: "ddcore", SecretKey: "ddcore-secret",
		Prefix: "apitest/" + strings.ToLower(t.Name()), PathStyle: true, PresignTTL: time.Minute,
	}}
}

func (x *env) stored(fileURL string) bool {
	x.t.Helper()
	key, ok := storage.KeyFromURL(fileURL)
	if !ok {
		x.t.Fatalf("not a file url: %s", fileURL)
	}
	rc, _, err := x.e.Storage().Open(context.Background(), key)
	if errors.Is(err, storage.ErrNotFound) {
		return false
	}
	if err != nil {
		x.t.Fatal(err)
	}
	rc.Close()
	return true
}

// PRD-05: the bytes of a File go when the File goes, whether it is deleted on
// its own or along with the document it is attached to.
func TestPRD05_DeletingAFileDeletesItsBytes(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Com Anexo"}, admin)
	if r.Status != 200 {
		t.Fatalf("create Pessoa: %d %s", r.Status, r.Raw)
	}
	pessoa := fmt.Sprint(r.Body["data"].(map[string]any)["id"])

	up := x.uploadAttachment(admin, "Pessoa", pessoa, "nota.pdf", "anexo")
	if up.Status != 200 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	attached := fmt.Sprint(up.Body["data"].(map[string]any)["file_url"])
	got := x.call("GET", attached, nil, admin)
	if got.Status != 200 || got.Raw != "anexo" {
		t.Fatalf("download: %d %s", got.Status, got.Raw)
	}
	if cd := got.Header.Get("Content-Disposition"); cd != `inline; filename=`+attached[len("/private/files/"):] {
		t.Errorf("Content-Disposition: %q", cd)
	}

	up = x.upload(admin, "solto.txt", "solto")
	if up.Status != 200 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	data := up.Body["data"].(map[string]any)
	detached, fileName := fmt.Sprint(data["file_url"]), fmt.Sprint(data["id"])
	if !x.stored(detached) {
		t.Fatal("upload did not store the bytes")
	}

	if r := x.call("DELETE", "/api/resource/File/"+fileName, nil, admin); r.Status != 200 {
		t.Fatalf("delete File: %d %s", r.Status, r.Raw)
	}
	if x.stored(detached) {
		t.Error("deleting a File left its bytes behind")
	}
	// a private url with no row behind it is refused, not reported missing
	if r := x.call("GET", detached, nil, admin); r.Status != 403 {
		t.Errorf("deleted file still served: %d", r.Status)
	}

	if r := x.call("DELETE", "/api/resource/Pessoa/"+pessoa, nil, admin); r.Status != 200 {
		t.Fatalf("delete Pessoa: %d %s", r.Status, r.Raw)
	}
	if x.stored(attached) {
		t.Error("deleting a document left its attachment's bytes behind")
	}
}

// A bare /files/ used to be an http.FileServer directory listing, which gave
// away every public file name the random names were meant to hide.
func TestPRD05_PublicFilesAreNotListed(t *testing.T) {
	x := setup(t)
	ana := "sid:" + x.sid("ana@x.com")
	if r := x.upload(ana, "public.png", "png"); r.Status != 200 {
		t.Fatalf("upload: %d %s", r.Status, r.Raw)
	}
	for _, p := range []string{"/files/", "/files/.", "/files/..%2fprivate"} {
		if r := x.call("GET", p, nil, ""); r.Status != 404 {
			t.Errorf("GET %s: %d %s", p, r.Status, r.Raw)
		}
	}
}
