package engine

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSource(t *testing.T, m *ExportManifest, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if m != nil {
		b, err := json.Marshal(m)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "manifest.json"), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func manifestFor(dt, file string, rows int64) *ExportManifest {
	return &ExportManifest{DDCore: "v0.17.0", Layout: ExportLayout, Format: "ndjson", Exports: []*ExportResult{{
		Summary: &ExportSummary{Doctype: dt, Rows: rows},
		Outputs: []ExportOutput{{File: file}},
	}}}
}

func readAllDocs(t *testing.T, r *ImportReader) []Doc {
	t.Helper()
	var out []Doc
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		out = append(out, rec.Doc)
	}
}

func TestImportReaderWalksLinesAndStopsAtTheManifest(t *testing.T) {
	body := `{"doctype":"Nota","id":"N-1","valor":1.5}
{"doctype":"Nota","id":"N-2","itens":[{"doctype":"Item Nota","id":"i1","idx":1}]}
{"_manifest":{"doctype":"Nota","rows":2}}
`
	dir := writeSource(t, manifestFor("Nota", "Nota.ndjson", 2), map[string]string{"Nota.ndjson": body})
	src, err := OpenImportSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := src.Open("Nota")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	docs := readAllDocs(t, r)
	if len(docs) != 2 || docs[0]["id"] != "N-1" {
		t.Fatalf("docs = %v", docs)
	}
	if r.Summary() == nil || r.Summary().Rows != 2 {
		t.Fatalf("summary = %+v", r.Summary())
	}
}

// The trailing _manifest line is the only proof the export was not cut short.
func TestImportReaderRefusesATruncatedFile(t *testing.T) {
	body := `{"doctype":"Nota","id":"N-1"}` + "\n"
	dir := writeSource(t, manifestFor("Nota", "Nota.ndjson", 1), map[string]string{"Nota.ndjson": body})
	src, err := OpenImportSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := src.Open("Nota")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	for {
		_, err := r.Next()
		if err == nil {
			continue
		}
		if errors.Is(err, io.EOF) {
			t.Fatal("a file without its manifest line read as complete")
		}
		if !strings.Contains(err.Error(), "truncated") {
			t.Fatalf("err = %v", err)
		}
		return
	}
}

// An export taken before the key rename says "name" where the site now says
// "id" — these are exactly the archives a migration has on hand.
func TestImportReaderTranslatesAPre017Export(t *testing.T) {
	body := `{"doctype":"Comment","name":"c1","reference_name":"N-1","naming_series":"C-","content":"hi"}
{"_manifest":{"doctype":"Comment","rows":1}}
`
	m := manifestFor("Comment", "Comment.ndjson", 1)
	m.DDCore = "v0.16.0"
	m.Layout = 0
	dir := writeSource(t, m, map[string]string{"Comment.ndjson": body})
	src, err := OpenImportSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !src.Legacy {
		t.Fatal("a 0.16 export should be read as legacy")
	}
	r, err := src.Open("Comment")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	docs := readAllDocs(t, r)
	d := docs[0]
	if d["id"] != "c1" || d["reference_id"] != "N-1" || d["id_series"] != "C-" {
		t.Fatalf("doc = %v", d)
	}
	if _, ok := d["name"]; ok {
		t.Fatalf("the old key survived: %v", d)
	}
}

// Resuming means starting again at the line after the last one that committed.
func TestImportReaderSkipsToALine(t *testing.T) {
	body := `{"doctype":"Nota","id":"N-1"}
{"doctype":"Nota","id":"N-2"}
{"doctype":"Nota","id":"N-3"}
{"_manifest":{"doctype":"Nota","rows":3}}
`
	dir := writeSource(t, manifestFor("Nota", "Nota.ndjson", 3), map[string]string{"Nota.ndjson": body})
	src, err := OpenImportSource(dir)
	if err != nil {
		t.Fatal(err)
	}
	r, err := src.Open("Nota")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := r.SkipTo(2); err != nil {
		t.Fatal(err)
	}
	rec, err := r.Next()
	if err != nil {
		t.Fatal(err)
	}
	if rec.Doc["id"] != "N-3" || rec.Line != 3 {
		t.Fatalf("rec = %+v", rec)
	}
}

// An Int beyond 2^53 has to survive the trip; json.Number keeps it.
func TestImportReaderKeepsLargeIntegers(t *testing.T) {
	body := `{"doctype":"Nota","id":"N-1","conta":9007199254740993}
{"_manifest":{"doctype":"Nota","rows":1}}
`
	dir := writeSource(t, manifestFor("Nota", "Nota.ndjson", 1), map[string]string{"Nota.ndjson": body})
	src, _ := OpenImportSource(dir)
	r, err := src.Open("Nota")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	docs := readAllDocs(t, r)
	if got, ok := docs[0]["conta"].(json.Number); !ok || got.String() != "9007199254740993" {
		t.Fatalf("conta = %#v", docs[0]["conta"])
	}
}

func TestOpenImportSourceRefusesADirectoryWithoutAManifest(t *testing.T) {
	if _, err := OpenImportSource(t.TempDir()); err == nil {
		t.Fatal("a directory without manifest.json is not an export")
	}
}
