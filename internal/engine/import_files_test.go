package engine

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/storage"
)

// attach writes an attachment into an export directory and returns the _files
// entry that names it, the way `ddcore export --attachments` does.
func attach(t *testing.T, dir, fileURL, body string) map[string]any {
	t.Helper()
	key, ok := storage.KeyFromURL(fileURL)
	if !ok {
		t.Fatalf("bad url %q", fileURL)
	}
	path := filepath.Join(dir, "files", filepath.FromSlash(key))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(body))
	return map[string]any{
		"id": "F-" + filepath.Base(fileURL), "attachedTo": "C-001", "fileName": filepath.Base(fileURL),
		"fileUrl": fileURL, "size": len(body), "contentType": "text/plain",
		"private": key[:7] == "private", "sha256": hex.EncodeToString(sum[:]),
	}
}

func TestImportLoadsAttachmentsAndTheirBytes(t *testing.T) {
	e := setupImport(t)
	docs := clientes(1)
	dir := writeExport(t, map[string][]Doc{"Cliente": docs})
	f := attach(t, dir, "/private/files/nota.txt", "hello")
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })

	run := runImport(t, e, dir, ImportArgs{})
	if run.Status != ImportCompleted {
		t.Fatalf("status = %s (%s) %+v", run.Status, run.Message, run.Errors)
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_file WHERE attached_to_id = $1", "C-001"); got != int64(1) {
		t.Fatalf("File rows = %v", got)
	}
	if got := scalar(t, e, "SELECT file_url FROM tab_file WHERE attached_to_id = $1", "C-001"); got != "/private/files/nota.txt" {
		t.Fatalf("file_url = %v; an import keeps the url, so every link to it still works", got)
	}
	b, err := storage.ReadAll(context.Background(), e.Storage(), "private/nota.txt")
	if err != nil || string(b) != "hello" {
		t.Fatalf("bytes = %q err = %v", b, err)
	}
}

// The checksum is what the manifest is for: bytes that do not match it are not
// the bytes the export took.
func TestImportRefusesBytesThatDoNotMatchTheirChecksum(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	f := attach(t, dir, "/private/files/nota.txt", "hello")
	f["sha256"] = "0000000000000000000000000000000000000000000000000000000000000000"
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })

	run := runImport(t, e, dir, ImportArgs{})
	if run.Status != ImportCompletedWithErrors {
		t.Fatalf("status = %s", run.Status)
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_file"); got != int64(0) {
		t.Fatalf("File rows = %v", got)
	}
}

// A file the export could not read is a finding for the reconciliation, not a
// reason to lose the document it belongs to.
func TestImportKeepsTheDocumentWhenAFileIsMissing(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	f := map[string]any{"id": "F-1", "attachedTo": "C-001", "fileName": "gone.txt",
		"fileUrl": "/private/files/gone.txt", "missing": true, "private": true}
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })

	run := runImport(t, e, dir, ImportArgs{})
	if run.Counts["Cliente"].Loaded != 1 {
		t.Fatalf("counts = %+v", run.Counts["Cliente"])
	}
	if len(run.Notes) == 0 {
		t.Fatal("a missing file has to be reported")
	}
}

// A public .html would be served from this site's own origin.
func TestImportRefusesDangerousPublicBytes(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	f := attach(t, dir, "/files/page.html", "<script>alert(1)</script>")
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })

	run := runImport(t, e, dir, ImportArgs{})
	if run.Counts["Cliente"].Errors != 1 {
		t.Fatalf("counts = %+v", run.Counts["Cliente"])
	}
}

// A second run must not attach the same file twice.
func TestImportDoesNotDuplicateAttachments(t *testing.T) {
	e := setupImport(t)
	dir := writeExport(t, map[string][]Doc{"Cliente": clientes(1)})
	f := attach(t, dir, "/private/files/nota.txt", "hello")
	rewriteLine(t, dir, "Cliente.ndjson", 0, func(d Doc) { d["_files"] = []any{f} })
	runImport(t, e, dir, ImportArgs{})
	runImport(t, e, dir, ImportArgs{})
	if got := scalar(t, e, "SELECT count(*) FROM tab_file"); got != int64(1) {
		t.Fatalf("File rows = %v", got)
	}
}

// rewriteLine edits one data line of an exported file in place.
func rewriteLine(t *testing.T, dir, file string, line int, fn func(Doc)) {
	t.Helper()
	path := filepath.Join(dir, file)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []byte
	n := 0
	for _, raw := range splitLines(b) {
		if n == line {
			var d Doc
			if err := json.Unmarshal(raw, &d); err != nil {
				t.Fatal(err)
			}
			if _, isManifest := d["_manifest"]; !isManifest {
				fn(d)
				raw, err = json.Marshal(d)
				if err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, isManifest := jsonHas(raw, "_manifest"); !isManifest {
			n++
		}
		out = append(append(out, raw...), '\n')
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		t.Fatal(err)
	}
}

func splitLines(b []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, c := range b {
		if c == '\n' {
			if i > start {
				out = append(out, b[start:i])
			}
			start = i + 1
		}
	}
	return out
}

func jsonHas(raw []byte, key string) (any, bool) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, false
	}
	v, ok := m[key]
	return v, ok
}
