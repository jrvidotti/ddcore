package storage

import (
	"bufio"
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/config"
)

// fakeS3 is the part of the S3 API the S3 store speaks — HEAD, GET (with
// Range), PUT, DELETE of an object, ListObjectsV2 and the multi-object delete
// — over path-style URLs, keeping objects in memory. It checks no signature.
// It lets the S3 store's own logic run in every `go test`, without the MinIO
// that TestS3 needs.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string]fakeObject // "<bucket>/<key>"
}

type fakeObject struct {
	data     []byte
	modified time.Time
	etag     string
}

func newFakeS3(t *testing.T) (*fakeS3, string) {
	f := &fakeS3{objects: map[string]fakeObject{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, strings.TrimPrefix(srv.URL, "http://")
}

// store is an S3 store on the fake with its own prefix and, when fallback is
// not nil, a read fallback to that prefix.
func (f *fakeS3) store(t *testing.T, endpoint, prefix string, fallback *string) *S3 {
	t.Helper()
	cfg := config.S3{Endpoint: endpoint, Bucket: "ddcore", Region: "us-east-1", AccessKey: "k", SecretKey: "s",
		Prefix: prefix, PathStyle: true, PresignTTL: time.Minute}
	if fallback != nil {
		cfg.Fallback, cfg.FallbackPrefix = true, *fallback
	}
	s, err := NewS3(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *fakeS3) has(name string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.objects["ddcore/"+name]
	return ok
}

func (f *fakeS3) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	bucket, key, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	q := r.URL.Query()
	switch {
	case key == "" && r.Method == http.MethodGet && q.Get("list-type") == "2":
		f.list(w, bucket, q.Get("prefix"))
	case key == "" && r.Method == http.MethodPost && q.Has("delete"):
		f.deleteMany(w, r, bucket)
	case key == "":
		http.Error(w, "unsupported bucket request", http.StatusNotImplemented)
	case r.Method == http.MethodPut:
		body, err := readBody(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		sum := md5.Sum(body)
		obj := fakeObject{data: body, modified: time.Now().UTC().Truncate(time.Second), etag: hex.EncodeToString(sum[:])}
		f.mu.Lock()
		f.objects[bucket+"/"+key] = obj
		f.mu.Unlock()
		w.Header().Set("ETag", `"`+obj.etag+`"`)
	case r.Method == http.MethodDelete:
		f.mu.Lock()
		delete(f.objects, bucket+"/"+key)
		f.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodHead || r.Method == http.MethodGet:
		f.mu.Lock()
		obj, ok := f.objects[bucket+"/"+key]
		f.mu.Unlock()
		if !ok {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusNotFound)
			if r.Method == http.MethodGet {
				fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>NoSuchKey</Code><Message>no such key</Message><Key>%s</Key></Error>`, key)
			}
			return
		}
		h := w.Header()
		h.Set("ETag", `"`+obj.etag+`"`)
		h.Set("Content-Type", "application/octet-stream")
		if v := q.Get("response-content-type"); v != "" {
			h.Set("Content-Type", v)
		}
		if v := q.Get("response-content-disposition"); v != "" {
			h.Set("Content-Disposition", v)
		}
		http.ServeContent(w, r, "", obj.modified, bytes.NewReader(obj.data))
	default:
		http.Error(w, "unsupported", http.StatusNotImplemented)
	}
}

// readBody undoes the aws-chunked encoding minio-go streams a PUT with over a
// plain-HTTP connection: "<hex size>[;chunk-signature=…]\r\n<bytes>\r\n",
// ending with a zero-size chunk and optional trailers.
func readBody(r *http.Request) ([]byte, error) {
	if !strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING") && !strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") {
		return io.ReadAll(r.Body)
	}
	br := bufio.NewReader(r.Body)
	var out bytes.Buffer
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeHex, _, _ := strings.Cut(strings.TrimSpace(line), ";")
		n, err := strconv.ParseInt(sizeHex, 16, 64)
		if err != nil {
			return nil, fmt.Errorf("chunk size %q: %w", line, err)
		}
		if n == 0 {
			return out.Bytes(), nil
		}
		if _, err := io.CopyN(&out, br, n); err != nil {
			return nil, err
		}
		if _, err := br.ReadString('\n'); err != nil {
			return nil, err
		}
	}
}

func (f *fakeS3) list(w http.ResponseWriter, bucket, prefix string) {
	type content struct {
		Key          string
		LastModified string
		ETag         string
		Size         int64
		StorageClass string
	}
	res := struct {
		XMLName     xml.Name `xml:"ListBucketResult"`
		Name        string
		Prefix      string
		KeyCount    int
		MaxKeys     int
		IsTruncated bool
		Contents    []content
	}{Name: bucket, Prefix: prefix, MaxKeys: 1000}
	f.mu.Lock()
	for name, obj := range f.objects {
		key, ok := strings.CutPrefix(name, bucket+"/")
		if ok && strings.HasPrefix(key, prefix) {
			res.Contents = append(res.Contents, content{Key: key, LastModified: obj.modified.Format("2006-01-02T15:04:05.000Z"),
				ETag: `"` + obj.etag + `"`, Size: int64(len(obj.data)), StorageClass: "STANDARD"})
		}
	}
	f.mu.Unlock()
	sort.Slice(res.Contents, func(i, j int) bool { return res.Contents[i].Key < res.Contents[j].Key })
	res.KeyCount = len(res.Contents)
	w.Header().Set("Content-Type", "application/xml")
	xml.NewEncoder(w).Encode(res)
}

func (f *fakeS3) deleteMany(w http.ResponseWriter, r *http.Request, bucket string) {
	var req struct {
		Objects []struct{ Key string } `xml:"Object"`
	}
	if err := xml.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	for _, o := range req.Objects {
		delete(f.objects, bucket+"/"+o.Key)
	}
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/xml")
	io.WriteString(w, `<?xml version="1.0" encoding="UTF-8"?><DeleteResult></DeleteResult>`)
}

// TestS3OnAFake runs the backend contract against the fake, so the S3 store's
// code is exercised on every run, not only where MinIO is up.
func TestS3OnAFake(t *testing.T) {
	f, endpoint := newFakeS3(t)
	s := f.store(t, endpoint, "test", nil)
	exercise(t, s)
	servePublic(t, s)

	ctx := t.Context()
	if err := s.Put(ctx, "private/p.pdf", strings.NewReader("pdf"), 3, "text/html"); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	if err := s.Serve(rec, httptest.NewRequest("GET", "/private/files/p.pdf", nil), "private/p.pdf", Serving{Name: "p.pdf", Inline: true}); err != nil {
		t.Fatal(err)
	}
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusFound || !strings.Contains(loc, "/ddcore/test/private/p.pdf?") ||
		rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("private Serve: %d %s %q", rec.Code, loc, rec.Header().Get("Cache-Control"))
	}
}

// #155: a development site on a copy of production's database reads
// production's objects through the fallback prefix, and never writes, deletes
// or lists them.
func TestS3FallbackPrefix(t *testing.T) {
	f, endpoint := newFakeS3(t)
	ctx := t.Context()
	prodPrefix := "prod"
	prod := f.store(t, endpoint, prodPrefix, nil)
	dev := f.store(t, endpoint, "dev", &prodPrefix)

	for key, body := range map[string]string{"public/photo.png": "prod photo", "private/contract.pdf": "prod pdf"} {
		if err := prod.Put(ctx, key, strings.NewReader(body), int64(len(body)), ""); err != nil {
			t.Fatal(err)
		}
	}

	// reads: Open, a streamed public file, a presigned private one
	if b, err := ReadAll(ctx, dev, "public/photo.png"); err != nil || string(b) != "prod photo" {
		t.Fatalf("dev read of a production file: %q %v", b, err)
	}
	rec := httptest.NewRecorder()
	if err := dev.Serve(rec, httptest.NewRequest("GET", "/files/photo.png", nil), "public/photo.png",
		Serving{Name: "photo.png", Inline: true, Public: true}); err != nil || rec.Code != 200 || rec.Body.String() != "prod photo" {
		t.Fatalf("dev public Serve: %v %d %q", err, rec.Code, rec.Body.String())
	}
	u, err := dev.Presign(ctx, "private/contract.pdf", time.Minute, Serving{Name: "contract.pdf"})
	if err != nil || !strings.Contains(u, "/ddcore/prod/private/contract.pdf?") {
		t.Fatalf("dev presign of a production file: %s %v", u, err)
	}
	res, err := http.Get(u)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(b) != "prod pdf" {
		t.Errorf("presigned GET: %d %q", res.StatusCode, b)
	}

	// the site's own object wins over the fallback
	if err := dev.Put(ctx, "public/photo.png", strings.NewReader("dev photo"), 9, ""); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadAll(ctx, dev, "public/photo.png"); err != nil || string(b) != "dev photo" {
		t.Errorf("dev's own object: %q %v", b, err)
	}
	if u, err := dev.Presign(ctx, "public/photo.png", time.Minute, Serving{Name: "photo.png"}); err != nil || !strings.Contains(u, "/ddcore/dev/public/photo.png?") {
		t.Errorf("presign of dev's own object: %s %v", u, err)
	}
	if !f.has("dev/public/photo.png") {
		t.Error("a write did not land under the site's own prefix")
	}

	// deletes and listings are the site's own
	if err := dev.Delete(ctx, "public/photo.png"); err != nil {
		t.Fatal(err)
	}
	if err := dev.Delete(ctx, "private/contract.pdf"); err != nil {
		t.Fatal(err)
	}
	if !f.has("prod/public/photo.png") || !f.has("prod/private/contract.pdf") {
		t.Fatal("a delete on dev removed production's object")
	}
	if b, err := ReadAll(ctx, dev, "public/photo.png"); err != nil || string(b) != "prod photo" {
		t.Errorf("after dev's delete, the fallback: %q %v", b, err)
	}
	if err := prod.Put(ctx, "thumbs/photo.png/64.jpg", strings.NewReader("t"), 1, ""); err != nil {
		t.Fatal(err)
	}
	if err := dev.DeletePrefix(ctx, "thumbs/photo.png/"); err != nil || !f.has("prod/thumbs/photo.png/64.jpg") {
		t.Errorf("DeletePrefix on dev reached production: %v", err)
	}
	if err := dev.List(ctx, "", func(key string, _ Info) error {
		t.Errorf("dev listed %s, which only production holds", key)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := dev.Open(ctx, "public/none.png"); err != ErrNotFound {
		t.Errorf("missing on both: %v", err)
	}

	// a production with no prefix: the fallback is the bucket root
	root := ""
	rootDev := f.store(t, endpoint, "dev2", &root)
	if err := f.store(t, endpoint, "", nil).Put(ctx, "public/root.png", strings.NewReader("root"), 4, ""); err != nil {
		t.Fatal(err)
	}
	if b, err := ReadAll(ctx, rootDev, "public/root.png"); err != nil || string(b) != "root" {
		t.Errorf("fallback to the bucket root: %q %v", b, err)
	}
}
