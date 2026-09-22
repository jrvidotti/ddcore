package engine

// The reading half of an import (DAT-01): an export directory opened for
// reading, and one NDJSON file walked line by line.
//
// Two things here are not obvious. The trailing `{"_manifest": …}` line is the
// only evidence the export was not cut short — a `200` had already been sent,
// so the writer had no status code left to fail with — and a reader that
// stopped at EOF would load a truncated file as if it were whole. And numbers
// are read as json.Number: an Int past 2^53 and a Currency's ninth decimal do
// not survive a float64, and this is the last place they are still exact.

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// ImportSource is an export directory, read through its manifest.
type ImportSource struct {
	Dir      string
	Manifest *ExportManifest
	// Legacy marks an export taken before the key rename (0.17), where the
	// document key is `name` and the reference columns end in `_name`. The
	// reader translates those to the current vocabulary.
	Legacy bool
}

// ImportRecord is one line of one file.
type ImportRecord struct {
	Doc  Doc
	Line int // 1-based, counting data lines only
}

// legacyColumns are the columns the 0.17 rename moved. `name` itself is handled
// apart, because an app may now declare an ordinary field called `name`.
var legacyColumns = map[string]string{
	"reference_name":   "reference_id",
	"share_name":       "share_id",
	"attached_to_name": "attached_to_id",
	"target_name":      "target_id",
	"docname":          "doc_id",
	"naming_series":    "id_series",
}

// OpenImportSource reads a directory's manifest.json and checks it describes an
// export this binary can read.
func OpenImportSource(dir string) (*ImportSource, error) {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, cerr.Validation("{0} is not an export directory: manifest.json is missing", dir)
		}
		return nil, err
	}
	var m ExportManifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, cerr.Validation("manifest.json is not readable: {0}", err)
	}
	if m.Format != "" && m.Format != "ndjson" {
		return nil, cerr.Validation("Only an ndjson export can be imported; this one is {0}", m.Format)
	}
	if len(m.Exports) == 0 {
		return nil, cerr.Validation("manifest.json lists no exports")
	}
	s := &ImportSource{Dir: dir, Manifest: &m}
	if v, ok := parseCoreVersion(m.DDCore); ok && v.cmp(semver{0, 17, 0}) < 0 {
		s.Legacy = true
	}
	return s, nil
}

// Doctypes is what the export holds, in the order it was written.
func (s *ImportSource) Doctypes() []string {
	var out []string
	for _, e := range s.Manifest.Exports {
		if e.Summary != nil {
			out = append(out, e.Summary.Doctype)
		}
	}
	return out
}

// Result is the manifest's entry for one DocType.
func (s *ImportSource) Result(doctype string) *ExportResult {
	for _, e := range s.Manifest.Exports {
		if e.Summary != nil && e.Summary.Doctype == doctype {
			return e
		}
	}
	return nil
}

// AttachmentPath is where one file's bytes sit inside the directory. Layout 2
// keys them by their storage key; layout 1 (and an export that declares none)
// by the base name, where a public and a private file sharing a name collide —
// a mismatched checksum is how that shows up.
func (s *ImportSource) AttachmentPath(fileURL, key string) string {
	if s.Manifest.Layout >= 2 && key != "" {
		return filepath.Join(s.Dir, "files", filepath.FromSlash(key))
	}
	return filepath.Join(s.Dir, "files", filepath.Base(fileURL))
}

// Open walks one DocType's file.
func (s *ImportSource) Open(doctype string) (*ImportReader, error) {
	res := s.Result(doctype)
	if res == nil {
		return nil, cerr.Validation("The export holds no {0}", doctype)
	}
	if len(res.Outputs) == 0 {
		return nil, cerr.Validation("The manifest records no file for {0}", doctype)
	}
	name := res.Outputs[0].File
	f, err := os.Open(filepath.Join(s.Dir, name))
	if err != nil {
		return nil, err
	}
	return &ImportReader{f: f, name: name, legacy: s.Legacy, br: bufio.NewReaderSize(f, 64*1024)}, nil
}

// ImportReader walks one NDJSON file. It is not safe for concurrent use.
type ImportReader struct {
	f       *os.File
	br      *bufio.Reader
	name    string
	legacy  bool
	line    int
	summary *ExportSummary
	done    bool
}

// Close releases the file.
func (r *ImportReader) Close() error { return r.f.Close() }

// Summary is the export's own count for this file, read from its last line. It
// is nil until the walk reaches that line.
func (r *ImportReader) Summary() *ExportSummary { return r.summary }

// SkipTo drops the first n data lines, which is how a resumed run starts again
// at the line after the last one that committed.
func (r *ImportReader) SkipTo(n int) error {
	for r.line < n {
		if _, err := r.Next(); err != nil {
			return err
		}
	}
	return nil
}

// Next reads the next document. It returns io.EOF only after the closing
// `_manifest` line; a file that ends without one is an error, because that is
// what a cut-short export looks like.
func (r *ImportReader) Next() (ImportRecord, error) {
	for {
		if r.done {
			return ImportRecord{}, io.EOF
		}
		line, err := r.readLine()
		if errors.Is(err, io.EOF) {
			return ImportRecord{}, cerr.Validation("{0} is truncated: it ends without its _manifest line, so the export did not finish", r.name)
		}
		if err != nil {
			return ImportRecord{}, err
		}
		if len(strings.TrimSpace(string(line))) == 0 {
			continue
		}
		dec := json.NewDecoder(strings.NewReader(string(line)))
		dec.UseNumber()
		var doc Doc
		if err := dec.Decode(&doc); err != nil {
			return ImportRecord{}, cerr.Validation("{0} line {1} is not a document: {2}", r.name, r.line+1, err)
		}
		if m, ok := doc["_manifest"]; ok {
			r.done = true
			r.summary = summaryFrom(m)
			return ImportRecord{}, io.EOF
		}
		r.line++
		if r.legacy {
			translateLegacy(doc)
		}
		return ImportRecord{Doc: doc, Line: r.line}, nil
	}
}

// readLine reads one whole line however long it is; bufio.Scanner would stop at
// 64 KiB, and a document with a Text field passes that without trying.
func (r *ImportReader) readLine() ([]byte, error) {
	var out []byte
	for {
		chunk, more, err := r.br.ReadLine()
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		if !more {
			return out, nil
		}
	}
}

// summaryFrom re-reads the trailing line's value as an ExportSummary. It went
// through json.Number on the way in, so it is marshalled back rather than cast.
func summaryFrom(v any) *ExportSummary {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var s ExportSummary
	if err := json.Unmarshal(b, &s); err != nil {
		return nil
	}
	return &s
}

// translateLegacy moves a pre-0.17 document to the current vocabulary: the key
// and the columns that held another document's key. It is the same translation
// `ddcore migrate` does to the columns themselves.
func translateLegacy(doc Doc) {
	for _, v := range doc {
		if rows, ok := v.([]any); ok {
			for _, row := range rows {
				if child, ok := row.(map[string]any); ok {
					translateLegacy(Doc(child))
				}
			}
		}
	}
	if _, has := doc["id"]; !has {
		if name, ok := doc["name"]; ok {
			doc["id"] = name
			delete(doc, "name")
		}
	}
	for old, now := range legacyColumns {
		v, ok := doc[old]
		if !ok {
			continue
		}
		if _, taken := doc[now]; !taken {
			doc[now] = v
		}
		delete(doc, old)
	}
	if files, ok := doc["_files"].([]any); ok {
		for _, f := range files {
			if m, ok := f.(map[string]any); ok {
				if v, has := m["name"]; has {
					if _, taken := m["id"]; !taken {
						m["id"] = v
					}
					delete(m, "name")
				}
			}
		}
	}
}
