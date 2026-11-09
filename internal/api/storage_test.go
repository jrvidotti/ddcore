package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
	"github.com/jrvidotti/ddcore/internal/engine"
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

// An upload is stored under a random name; /api/file-info gives the original
// back, with its size and type, to whoever may read the file.
func TestFileInfoFollowsTheAttachedDocument(t *testing.T) {
	x := setup(t)
	admin, ana, ze := "sid:"+x.sid("Admin"), "sid:"+x.sid("ana@x.com"), "sid:"+x.sid("ze@x.com")

	r := x.call("POST", "/api/resource/Pessoa", map[string]any{"nome": "Com Foto"}, admin)
	if r.Status != 200 {
		t.Fatalf("create Pessoa: %d %s", r.Status, r.Raw)
	}
	pessoa := fmt.Sprint(r.Body["data"].(map[string]any)["id"])
	up := x.uploadAttachment(admin, "Pessoa", pessoa, "contrato.pdf", "anexo")
	if up.Status != 200 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	fileURL := fmt.Sprint(up.Body["data"].(map[string]any)["file_url"])
	info := "/api/file-info?url=" + url.QueryEscape(fileURL)

	for _, auth := range []string{admin, ana} {
		got := x.call("GET", info, nil, auth)
		if got.Status != 200 {
			t.Fatalf("file-info: %d %s", got.Status, got.Raw)
		}
		d := got.Body["data"].(map[string]any)
		if d["file_name"] != "contrato.pdf" || fmt.Sprint(d["file_size"]) != "5" {
			t.Errorf("file-info: %v", d)
		}
	}
	x.expect(x.call("GET", info, nil, ze), 403, "PermissionError")
	// a url with no row behind it is refused, not reported missing
	x.expect(x.call("GET", "/api/file-info?url=/private/files/nada.pdf", nil, ana), 403, "PermissionError")
}

// uploadPublic stores a detached public file (is_private=0).
func (x *env) uploadPublic(auth, filename string, content []byte) string {
	x.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("is_private", "0")
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write(content)
	mw.Close()
	req, _ := http.NewRequest("POST", x.ts.URL+"/api/upload", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "test")
	req.AddCookie(&http.Cookie{Name: "sid", Value: strings.TrimPrefix(auth, "sid:")})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		x.t.Fatal(err)
	}
	defer res.Body.Close()
	var out struct {
		Data map[string]any `json:"data"`
	}
	raw, _ := io.ReadAll(res.Body)
	if err := json.Unmarshal(raw, &out); err != nil || res.StatusCode != 200 {
		x.t.Fatalf("upload %s: %d %s", filename, res.StatusCode, raw)
	}
	u := fmt.Sprint(out.Data["file_url"])
	if !strings.HasPrefix(u, "/files/") {
		x.t.Fatalf("upload %s is not public: %s", filename, u)
	}
	return u
}

const immutableCache = "public, max-age=31536000, immutable"

// #153: a public file is served by the server itself, on either backend, with
// a Cache-Control that lets a browser or a CDN keep it; a private one never
// is cached.
func TestPublicFilesAreCacheable(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	pub := x.uploadPublic(admin, "photo.png", []byte("png bytes"))

	r := x.call("GET", pub, nil, "")
	if r.Status != 200 || r.Raw != "png bytes" {
		t.Fatalf("GET %s as Guest: %d %q", pub, r.Status, r.Raw)
	}
	h := r.Header
	if h.Get("Cache-Control") != immutableCache || h.Get("X-Content-Type-Options") != "nosniff" ||
		!strings.Contains(h.Get("Content-Security-Policy"), "sandbox") || h.Get("Last-Modified") == "" {
		t.Errorf("public file headers: %v", h)
	}
	if r := x.call("GET", pub, nil, "", "If-Modified-Since", h.Get("Last-Modified")); r.Status != 304 {
		t.Errorf("conditional GET: %d", r.Status)
	}

	if r := x.call("GET", "/files/no-such-file.png", nil, ""); r.Status != 404 || strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("missing public file: %d %q", r.Status, r.Header.Get("Cache-Control"))
	}

	up := x.upload(admin, "secret.pdf", "secret")
	if up.Status != 200 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	priv := fmt.Sprint(up.Body["data"].(map[string]any)["file_url"])
	if r := x.call("GET", priv, nil, admin); r.Status != 200 || strings.Contains(r.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("private file: %d %q", r.Status, r.Header.Get("Cache-Control"))
	}
}

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.NRGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

// #154: /files/<image>?w=<n> is a cached, stored thumbnail of a public image,
// in one of a few sizes, and goes when the File goes.
func TestThumbnails(t *testing.T) {
	x := setup(t)
	admin := "sid:" + x.sid("Admin")
	src := testPNG(t, 600, 300)
	pub := x.uploadPublic(admin, "product.png", src)
	key, _ := storage.KeyFromURL(pub)
	thumbKey := storage.ThumbKey(key, 128, ".jpg")

	for i := 0; i < 2; i++ { // made, then read back from the store
		r := x.call("GET", pub+"?w=128", nil, "")
		if r.Status != 200 || r.Header.Get("Content-Type") != "image/jpeg" || r.Header.Get("Cache-Control") != immutableCache {
			t.Fatalf("thumbnail #%d: %d %v", i, r.Status, r.Header)
		}
		cfg, format, err := image.DecodeConfig(strings.NewReader(r.Raw))
		if err != nil || format != "jpeg" || cfg.Width != 128 || cfg.Height != 64 {
			t.Fatalf("thumbnail #%d: %s %dx%d %v", i, format, cfg.Width, cfg.Height, err)
		}
		if len(r.Raw) >= len(src) {
			t.Errorf("thumbnail #%d is %d bytes, the original %d", i, len(r.Raw), len(src))
		}
		if !strings.HasPrefix(r.Header.Get("Content-Disposition"), "inline") {
			t.Errorf("thumbnail disposition: %q", r.Header.Get("Content-Disposition"))
		}
		if i == 0 && !x.storedKey(thumbKey) {
			t.Fatal("the thumbnail was not stored")
		}
	}

	if r := x.call("GET", pub+"?w=100", nil, ""); r.Status != 400 {
		t.Errorf("w=100: %d %s", r.Status, r.Raw)
	}
	if r := x.call("GET", "/files/no-such.png?w=128", nil, ""); r.Status != 404 {
		t.Errorf("thumbnail of a missing file: %d", r.Status)
	}
	// on anything but a public image, w is ignored
	txt := x.uploadPublic(admin, "notes.txt", []byte("plain"))
	if r := x.call("GET", txt+"?w=128", nil, ""); r.Status != 200 || r.Raw != "plain" {
		t.Errorf("non-image ?w: %d %q", r.Status, r.Raw)
	}
	broken := x.uploadPublic(admin, "broken.png", []byte("not a png"))
	if r := x.call("GET", broken+"?w=64", nil, ""); r.Status != 200 || r.Raw != "not a png" {
		t.Errorf("undecodable image ?w: %d %q", r.Status, r.Raw)
	}
	up := x.upload(admin, "private.png", string(src))
	if up.Status != 200 {
		t.Fatalf("upload: %d %s", up.Status, up.Raw)
	}
	priv := fmt.Sprint(up.Body["data"].(map[string]any)["file_url"])
	if r := x.call("GET", priv+"?w=128", nil, admin); r.Status != 200 || r.Raw != string(src) {
		t.Errorf("private ?w: %d, %d bytes", r.Status, len(r.Raw))
	}

	// deleting the File takes its thumbnails
	var id string
	if err := x.e.Run(context.Background(), "Admin", func(c *engine.Ctx) error {
		f, err := c.GetValues("File", map[string]any{"file_url": pub}, []string{"id"})
		if err == nil && f != nil {
			id = fmt.Sprint(f["id"])
		}
		return err
	}); err != nil || id == "" {
		t.Fatalf("File for %s: %q %v", pub, id, err)
	}
	if r := x.call("DELETE", "/api/resource/File/"+id, nil, admin); r.Status != 200 {
		t.Fatalf("delete File: %d %s", r.Status, r.Raw)
	}
	if x.storedKey(thumbKey) {
		t.Error("deleting a File left its thumbnail behind")
	}
}

func (x *env) storedKey(key string) bool {
	x.t.Helper()
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
