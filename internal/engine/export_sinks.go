package engine

// The two shapes an export comes out in.
//
// NDJSON is the reconcilable one: one document per line with its children
// nested, every audit column present, and a trailing manifest line. CSV is the
// one a person opens in a spreadsheet — and it follows exactly the rules
// desk/src/lib/csv.ts documents, because the same people compare the two files.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// ---------------------------------------------------------------- NDJSON

// NDJSONSink writes one JSON object per line and closes with
// `{"_manifest": …}`. A consumer that does not see the manifest line knows the
// stream was cut short — the only way to tell, once a 200 has been sent.
type NDJSONSink struct {
	W        io.Writer
	Manifest bool // write the trailing manifest line (off for a plain dump)
	enc      *json.Encoder
}

func NewNDJSONSink(w io.Writer) *NDJSONSink {
	return &NDJSONSink{W: w, Manifest: true, enc: json.NewEncoder(w)}
}

func (s *NDJSONSink) Begin(*meta.DocType, []string) error { return nil }

func (s *NDJSONSink) Doc(doc Doc, files []ExportFile) error {
	if len(files) > 0 {
		doc["_files"] = files
	}
	return s.enc.Encode(doc)
}

func (s *NDJSONSink) End(sum *ExportSummary) error {
	if !s.Manifest {
		return nil
	}
	return s.enc.Encode(map[string]any{"_manifest": sum})
}

// ---------------------------------------------------------------- CSV

// CSVSink writes one CSV per table: the parent under its own name, each child
// table under "<Doctype>.<parentfield>", and the attachments under
// "<Doctype>.files". Open is called at most once per table, the first time a
// row needs it, so an export with no attachments leaves no empty file behind.
type CSVSink struct {
	Open func(table string) (io.Writer, error)
	Sep  rune // 0 = comma
	// Lookup resolves a child doctype's metadata. The sink writes the child
	// tables too, and is handed the lookup rather than the whole engine.
	Lookup func(name string) (*meta.DocType, error)

	d       *meta.DocType
	columns []string
	parent  *csv.Writer
	child   map[string]*csv.Writer
	files   *csv.Writer
	flush   []*csv.Writer
}

var exportFileColumns = []string{"name", "attached_to", "field", "file_name", "file_url", "size", "content_type", "is_private", "sha256", "missing"}

func (s *CSVSink) Begin(d *meta.DocType, columns []string) error {
	s.d, s.columns, s.child = d, columns, map[string]*csv.Writer{}
	return nil
}

// writer opens `table` and writes its header. The UTF-8 BOM goes in first:
// without it Excel reads "Endereço" as "EndereÃ§o".
func (s *CSVSink) writer(table string, header []string) (*csv.Writer, error) {
	w, err := s.Open(table)
	if err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return nil, err
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true // what spreadsheets expect
	if s.Sep != 0 {
		cw.Comma = s.Sep
	}
	if err := cw.Write(header); err != nil {
		return nil, err
	}
	s.flush = append(s.flush, cw)
	return cw, nil
}

func (s *CSVSink) Doc(doc Doc, files []ExportFile) error {
	if s.parent == nil {
		w, err := s.writer(s.d.Name, s.columns)
		if err != nil {
			return err
		}
		s.parent = w
	}
	if err := s.parent.Write(csvRow(doc, s.columns)); err != nil {
		return err
	}
	for _, tf := range s.d.TableFields() {
		rows := doc.Children(tf.Fieldname)
		if len(rows) == 0 {
			continue
		}
		child, err := s.childMeta(tf.OptionsString())
		if err != nil {
			return err
		}
		cols := ExportColumns(child)
		cw := s.child[tf.Fieldname]
		if cw == nil {
			if cw, err = s.writer(s.d.Name+"."+tf.Fieldname, cols); err != nil {
				return err
			}
			s.child[tf.Fieldname] = cw
		}
		for _, r := range rows {
			if err := cw.Write(csvRow(r, cols)); err != nil {
				return err
			}
		}
	}
	for _, f := range files {
		if s.files == nil {
			w, err := s.writer(s.d.Name+".files", exportFileColumns)
			if err != nil {
				return err
			}
			s.files = w
		}
		if err := s.files.Write([]string{
			f.Name, f.AttachedTo, f.Field, f.FileName, f.FileURL,
			strconv.FormatInt(f.Size, 10), f.ContentType,
			strconv.FormatBool(f.Private), f.SHA256, strconv.FormatBool(f.Missing),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *CSVSink) End(*ExportSummary) error {
	for _, w := range s.flush {
		w.Flush()
		if err := w.Error(); err != nil {
			return err
		}
	}
	return nil
}

func (s *CSVSink) childMeta(name string) (*meta.DocType, error) {
	if s.Lookup == nil {
		return nil, fmt.Errorf("CSVSink: no doctype lookup")
	}
	return s.Lookup(name)
}

func csvRow(doc Doc, columns []string) []string {
	row := make([]string, len(columns))
	for i, c := range columns {
		row[i] = csvCell(doc[c])
	}
	return row
}

// csvCell renders one value the way the JSON API renders it, so a CSV cell and
// the matching NDJSON field never disagree: a Check is "true", not "1", and a
// JSON column keeps its compact JSON. db.Rows has already flattened dates and
// numerics to strings and float64, so no time.Time reaches here.
func csvCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case bool:
		return strconv.FormatBool(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int64:
		return strconv.FormatInt(x, 10)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(strings.TrimPrefix(string(b), `"`), `"`)
}
