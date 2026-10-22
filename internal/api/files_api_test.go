package api

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/storage"
)

// evalAs runs JavaScript as user in a transaction of its own and returns the
// result decoded. rollback makes the transaction fail after the code ran.
func (x *env) evalAs(user, code string, rollback bool) (any, error) {
	var out any
	err := x.as(user, func(c *engine.Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		raw, err := rt.Eval(code)
		if err != nil {
			return err
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return err
		}
		if rollback {
			return errRolledBack
		}
		return nil
	})
	return out, err
}

var errRolledBack = errors.New("rolled back on purpose")

func (x *env) newPessoa(nome string) {
	x.t.Helper()
	x.asAdmin(func(c *engine.Ctx) error {
		d, err := c.NewDoc("Pessoa", engine.Doc{"nome": nome})
		if err != nil {
			return err
		}
		_, err = c.Insert(d, engine.SaveOpts{})
		return err
	})
}

func TestFilesSaveFromContent(t *testing.T) {
	x := setup(t)
	x.newPessoa("Com Arquivo")

	// "ÿ\u0000" is not UTF-8 once decoded: base64 is how bytes get in intact
	v, err := x.evalAs("Admin", `ddcore.files.save({doctype: "Pessoa", id: "Com Arquivo", filename: "a.png",
		contentBase64: "iVBORw0KGgoA//6A"})`, false)
	if err != nil {
		t.Fatal(err)
	}
	f := v.(map[string]any)
	url := fmt.Sprint(f["file_url"])
	if !strings.HasPrefix(url, "/private/files/") || !strings.HasSuffix(url, ".png") || f["attached_to_id"] != "Com Arquivo" {
		t.Fatalf("file: %v", f)
	}
	if f["content_type"] != "image/png" || f["file_size"] != float64(12) || f["file_name"] != "a.png" {
		t.Fatalf("metadata: %v", f)
	}
	if !x.stored(url) {
		t.Fatal("bytes not stored")
	}

	v, err = x.evalAs("Admin", `ddcore.files.save({filename: "page.html", content: "<script>", isPrivate: false})`, false)
	if err != nil {
		t.Fatal(err)
	}
	if url := fmt.Sprint(v.(map[string]any)["file_url"]); !strings.HasPrefix(url, "/files/") || !strings.HasSuffix(url, ".bin") {
		t.Fatalf("a public html file keeps no executable extension: %s", url)
	}

	for code, want := range map[string]string{
		`ddcore.files.save({filename: "a.txt"})`:                                                 "exactly one",
		`ddcore.files.save({filename: "a.txt", content: "x", contentBase64: "eA=="})`:            "exactly one",
		`ddcore.files.save({content: "x"})`:                                                      "filename",
		`ddcore.files.save({filename: "a.txt", content: "12345", maxBytes: 4})`:                  "larger than",
		`ddcore.files.save({filename: "a.txt", contentBase64: "not base64!"})`:                   "base64",
		`ddcore.files.save({doctype: "Pessoa", id: "Com Arquivo", filename: "a", content: "x"})`: "",
	} {
		_, err := x.evalAs("ze@x.com", code, false)
		if want == "" {
			want = "permission"
		}
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(want)) {
			t.Errorf("%s: want %q, got %v", code, want, err)
		}
	}
	// ze cannot write Pessoa; server code that must attach anyway says so
	if _, err := x.evalAs("ze@x.com", `ddcore.files.save({doctype: "Pessoa", id: "Com Arquivo", filename: "a.txt",
		content: "x", ignorePermissions: true})`, false); err != nil {
		t.Fatalf("ignorePermissions: %v", err)
	}
}

// The File row is on the caller's transaction; the bytes are not, so a
// rollback has to take them back out.
func TestFilesSaveRollbackDeletesBytes(t *testing.T) {
	x := setup(t)
	v, err := x.evalAs("Admin", `ddcore.files.save({filename: "a.txt", content: "gone"})`, true)
	if !errors.Is(err, errRolledBack) {
		t.Fatalf("err = %v", err)
	}
	url := fmt.Sprint(v.(map[string]any)["file_url"])
	if x.stored(url) {
		t.Fatal("bytes survived the rollback")
	}
	x.asAdmin(func(c *engine.Ctx) error {
		n, err := db.Select(c.Ctx, c.Q(), "SELECT 1 FROM tab_file WHERE file_url = $1", url)
		if err == nil && len(n) != 0 {
			t.Fatal("row survived the rollback")
		}
		return err
	})
}

func TestFilesSaveFromURL(t *testing.T) {
	x := setup(t)
	audio := []byte{'O', 'g', 'g', 'S', 0x00, 0xff, 0x80}
	src := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "audio/ogg")
		w.Write(audio)
	}))
	defer src.Close()

	v, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.save({fromUrl: %q, headers: {Authorization: "Bearer tok"}, timeout: 5})`,
		src.URL+"/media/voice.ogg"), false)
	if err != nil {
		t.Fatal(err)
	}
	f := v.(map[string]any)
	if f["file_name"] != "voice.ogg" || f["content_type"] != "audio/ogg" || f["file_size"] != float64(len(audio)) {
		t.Fatalf("metadata: %v", f)
	}
	got := x.read(fmt.Sprint(f["file_url"]))
	if string(got) != string(audio) {
		t.Fatalf("bytes %v, want %v", got, audio)
	}
	if _, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.save({fromUrl: %q})`, src.URL+"/a.ogg"), false); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("an error status is an error: %v", err)
	}
	if _, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.save({fromUrl: %q, headers: {Authorization: "Bearer tok"}, maxBytes: 3})`, src.URL+"/a.ogg"), false); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("a body over maxBytes is an error: %v", err)
	}
}

func TestFilesPresign(t *testing.T) {
	x := setup(t)
	v, err := x.evalAs("Admin", `ddcore.files.save({filename: "a.txt", content: "for a third party"})`, false)
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprint(v.(map[string]any)["file_url"])
	code := fmt.Sprintf(`ddcore.files.presign(%q, {ttl: 60})`, url)

	// the file is detached and Admin's: ze may not read it, so may not hand it out
	if _, err := x.evalAs("ze@x.com", code, false); err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Fatalf("ze: %v", err)
	}
	if _, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.presign(%q, {ttl: 8 * 86400})`, url), false); err == nil {
		t.Fatal("a ttl above 7 days is refused")
	}
	signed, err := x.evalAs("Admin", code, false)
	if x.e.Storage().Backend() == "local" {
		if err == nil || !strings.Contains(err.Error(), "s3") {
			t.Fatalf("local backend: want an s3 error, got %v", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.Get(fmt.Sprint(signed))
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || string(b) != "for a third party" {
		t.Fatalf("presigned GET: %d %q", res.StatusCode, b)
	}
}

// files.read hands back the bytes of a stored File, named by its url or its
// id, on whichever backend the test runs on, under the rule of /private/files.
func TestFilesRead(t *testing.T) {
	x := setup(t)
	bin := []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0xff}
	v, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.save({filename: "photo.png", contentBase64: %q})`, base64.StdEncoding.EncodeToString(bin)), false)
	if err != nil {
		t.Fatal(err)
	}
	f := v.(map[string]any)
	url, id := fmt.Sprint(f["file_url"]), fmt.Sprint(f["id"])
	for _, ref := range []string{url, id} {
		got, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.read(%q)`, ref), false)
		if err != nil {
			t.Fatalf("%s: %v", ref, err)
		}
		m := got.(map[string]any)
		if m["base64"] != base64.StdEncoding.EncodeToString(bin) || m["contentType"] != "image/png" ||
			m["size"] != float64(len(bin)) || m["fileName"] != "photo.png" {
			t.Fatalf("%s: %v", ref, m)
		}
	}
	if _, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.read(%q, {maxBytes: 4})`, url), false); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("a file over maxBytes is an error: %v", err)
	}
	// the file is detached and Admin's: ze may not read it, unless the code
	// reads on the system's behalf
	if _, err := x.evalAs("ze@x.com", fmt.Sprintf(`ddcore.files.read(%q)`, url), false); err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Fatalf("ze: %v", err)
	}
	if _, err := x.evalAs("ze@x.com", fmt.Sprintf(`ddcore.files.read(%q, {ignorePermissions: true})`, url), false); err != nil {
		t.Fatalf("ze with ignorePermissions: %v", err)
	}
	if _, err := x.evalAs("Admin", `ddcore.files.read("/private/files/nothing.png")`, false); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("an unknown file: %v", err)
	}
}

// files.sha256 is what sha256sum prints for the stored bytes, and what
// crypto.sha256 gives for them held as base64.
func TestFilesSha256(t *testing.T) {
	x := setup(t)
	bin := []byte("\x00\x01binary\xff photo")
	b64 := base64.StdEncoding.EncodeToString(bin)
	v, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.save({filename: "p.jpg", contentBase64: %q})`, b64), false)
	if err != nil {
		t.Fatal(err)
	}
	url := fmt.Sprint(v.(map[string]any)["file_url"])
	sum := sha256.Sum256(bin)
	got, err := x.evalAs("Admin", fmt.Sprintf(`[ddcore.files.sha256(%q), ddcore.files.sha256(%q, {output: "base64"}), ddcore.crypto.sha256(%q, {inputEncoding: "base64"})]`, url, url, b64), false)
	if err != nil {
		t.Fatal(err)
	}
	want := []any{hex.EncodeToString(sum[:]), base64.StdEncoding.EncodeToString(sum[:]), hex.EncodeToString(sum[:])}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if _, err := x.evalAs("ze@x.com", fmt.Sprintf(`ddcore.files.sha256(%q)`, url), false); err == nil || !strings.Contains(strings.ToLower(err.Error()), "permission") {
		t.Fatalf("ze: %v", err)
	}
	if _, err := x.evalAs("Admin", fmt.Sprintf(`ddcore.files.sha256(%q, {output: "bin"})`, url), false); err == nil {
		t.Fatal("an unknown output should throw")
	}
}

func (x *env) read(fileURL string) []byte {
	x.t.Helper()
	key, ok := storage.KeyFromURL(fileURL)
	if !ok {
		x.t.Fatalf("not a file url: %s", fileURL)
	}
	b, err := storage.ReadAll(x.ctx, x.e.Storage(), key)
	if err != nil {
		x.t.Fatal(err)
	}
	return b
}
