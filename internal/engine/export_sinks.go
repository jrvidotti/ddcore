package engine

// The three shapes an export comes out in.
//
// NDJSON is the reconcilable one: one document per line with its children
// nested, every audit column present, and a trailing manifest line. CSV is the
// one a person opens in a spreadsheet — and it follows exactly the rules
// desk/src/lib/csv.ts documents, because the same people compare the two files.
// XLSX holds the CSV's columns and values with their types kept, so a
// spreadsheet reads numbers and dates without asking the reader's locale.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/tabular"
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

// ---------------------------------------------------------------- tables

// tableWriter is one output table: a CSV file or a workbook. The cells are
// the values as the walk produced them; the writer renders them.
type tableWriter interface {
	Write(row []any) error
	Close() error
}

// tableSet is what the CSV and XLSX sinks share: one table per doctype — the
// parent under its own name, each child table under "<Doctype>.<parentfield>",
// the attachments under "<Doctype>.files" — each opened the first time a row
// needs it, so an export with no attachments leaves no empty file behind.
type tableSet struct {
	// open starts `table` with its header; d is the table's doctype (nil for
	// the attachments), so a writer that types its cells can read the fields.
	open   func(table string, d *meta.DocType, header []string) (tableWriter, error)
	lookup func(name string) (*meta.DocType, error)

	d       *meta.DocType
	columns []string
	parent  tableWriter
	child   map[string]tableWriter
	// childCols are the columns each child table was opened with.
	childCols map[string][]string
	files     tableWriter
	opened    []tableWriter
}

var exportFileColumns = []string{"id", "attached_to", "field", "file_name", "file_url", "size", "content_type", "is_private", "sha256", "missing"}

func (s *tableSet) begin(d *meta.DocType, columns []string) {
	s.d, s.columns, s.child, s.childCols = d, columns, map[string]tableWriter{}, map[string][]string{}
}

func (s *tableSet) writer(table string, d *meta.DocType, header []string) (tableWriter, error) {
	w, err := s.open(table, d, header)
	if err != nil {
		return nil, err
	}
	s.opened = append(s.opened, w)
	return w, nil
}

// openParent opens the parent table if no row has yet.
func (s *tableSet) openParent() error {
	if s.parent != nil {
		return nil
	}
	w, err := s.writer(s.d.Name, s.d, s.columns)
	s.parent = w
	return err
}

func (s *tableSet) doc(doc Doc, files []ExportFile) error {
	if err := s.openParent(); err != nil {
		return err
	}
	if err := s.parent.Write(rowOf(doc, s.columns)); err != nil {
		return err
	}
	for _, tf := range s.d.TableFields() {
		rows := doc.Children(tf.Fieldname)
		if len(rows) == 0 {
			continue
		}
		if s.lookup == nil {
			return fmt.Errorf("export sink: no doctype lookup")
		}
		child, err := s.lookup(tf.OptionsString())
		if err != nil {
			return err
		}
		cols := s.childCols[tf.Fieldname]
		cw := s.child[tf.Fieldname]
		if cw == nil {
			// a column the export left out for this user (a restricted
			// field) is left out of the header too, not written blank
			cols = nil
			for _, col := range ExportColumns(child) {
				if _, ok := rows[0][col]; ok {
					cols = append(cols, col)
				}
			}
			s.childCols[tf.Fieldname] = cols
			if cw, err = s.writer(s.d.Name+"."+tf.Fieldname, child, cols); err != nil {
				return err
			}
			s.child[tf.Fieldname] = cw
		}
		for _, r := range rows {
			if err := cw.Write(rowOf(r, cols)); err != nil {
				return err
			}
		}
	}
	for _, f := range files {
		if s.files == nil {
			w, err := s.writer(s.d.Name+".files", nil, exportFileColumns)
			if err != nil {
				return err
			}
			s.files = w
		}
		if err := s.files.Write([]any{
			f.ID, f.AttachedTo, f.Field, f.FileName, f.FileURL,
			f.Size, f.ContentType, f.Private, f.SHA256, f.Missing,
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *tableSet) end() error {
	for _, w := range s.opened {
		if err := w.Close(); err != nil {
			return err
		}
	}
	return nil
}

func rowOf(doc Doc, columns []string) []any {
	row := make([]any, len(columns))
	for i, c := range columns {
		row[i] = doc[c]
	}
	return row
}

// ---------------------------------------------------------------- CSV

// CSVSink writes one CSV per table (see tableSet). Open is called at most
// once per table.
type CSVSink struct {
	Open func(table string) (io.Writer, error)
	Sep  rune // 0 = comma
	// Lookup resolves a child doctype's metadata. The sink writes the child
	// tables too, and is handed the lookup rather than the whole engine.
	Lookup func(name string) (*meta.DocType, error)

	t tableSet
}

func (s *CSVSink) Begin(d *meta.DocType, columns []string) error {
	s.t = tableSet{open: s.openCSV, lookup: s.Lookup}
	s.t.begin(d, columns)
	return nil
}

func (s *CSVSink) Doc(doc Doc, files []ExportFile) error { return s.t.doc(doc, files) }

func (s *CSVSink) End(*ExportSummary) error { return s.t.end() }

// openCSV opens `table` and writes its header. The UTF-8 BOM goes in first
// (newCSVWriter): without it Excel reads "Endereço" as "EndereÃ§o".
func (s *CSVSink) openCSV(table string, _ *meta.DocType, header []string) (tableWriter, error) {
	w, err := s.Open(table)
	if err != nil {
		return nil, err
	}
	cw, err := newCSVWriter(w, s.Sep)
	if err != nil {
		return nil, err
	}
	if err := cw.Write(header); err != nil {
		return nil, err
	}
	return csvTable{cw}, nil
}

// newCSVWriter starts a CSV the way every CSV ddcore writes starts: the UTF-8
// BOM, CRLF line ends and the separator (0 = comma).
func newCSVWriter(w io.Writer, sep rune) (*csv.Writer, error) {
	if _, err := io.WriteString(w, "\ufeff"); err != nil {
		return nil, err
	}
	cw := csv.NewWriter(w)
	cw.UseCRLF = true // what spreadsheets expect
	if sep != 0 {
		cw.Comma = sep
	}
	return cw, nil
}

type csvTable struct{ w *csv.Writer }

func (t csvTable) Write(row []any) error {
	cells := make([]string, len(row))
	for i, v := range row {
		cells[i] = csvCell(v)
	}
	return t.w.Write(cells)
}

func (t csvTable) Close() error {
	t.w.Flush()
	return t.w.Error()
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

// ---------------------------------------------------------------- XLSX

// XLSXSink writes one workbook per table (see tableSet), streamed row by row.
// Its cells are typed, which is what a CSV cannot do: Currency, Float, Int and
// the other numbers are numbers, Check a boolean, Date and Time real dates and
// times, and Datetime a real date-time on the site's clock — what the desk
// shows, rather than the UTC instant the API carries. Everything else is the
// text the CSV holds, Select values and Link ids included.
//
// The parent's workbook is written even when no row matched: an empty CSV is
// still a file, an empty zip is not a workbook.
type XLSXSink struct {
	Open   func(table string) (io.Writer, error)
	Lookup func(name string) (*meta.DocType, error)
	// Loc is the site's timezone, the clock a Datetime is shown on; nil is UTC.
	Loc *time.Location
	// MoneyDecimals is how many decimals a Currency cell shows.
	MoneyDecimals int

	t tableSet
}

func (s *XLSXSink) Begin(d *meta.DocType, columns []string) error {
	s.t = tableSet{open: s.openXLSX, lookup: s.Lookup}
	s.t.begin(d, columns)
	return nil
}

func (s *XLSXSink) Doc(doc Doc, files []ExportFile) error { return s.t.doc(doc, files) }

func (s *XLSXSink) End(*ExportSummary) error {
	if err := s.t.openParent(); err != nil {
		return err
	}
	return s.t.end()
}

func (s *XLSXSink) openXLSX(table string, d *meta.DocType, header []string) (tableWriter, error) {
	w, err := s.Open(table)
	if err != nil {
		return nil, err
	}
	x, err := tabular.NewXLSXWriter(w, tabular.XLSXOptions{SheetName: table, MoneyDecimals: s.MoneyDecimals})
	if err != nil {
		return nil, err
	}
	head := make([]any, len(header))
	types := make([]string, len(header))
	places := make([]*int, len(header))
	for i, h := range header {
		head[i] = h
		types[i] = exportColumnType(d, h)
		if d != nil && types[i] == "Currency" {
			if f := d.Field(h); f != nil {
				places[i] = f.Precision
			}
		}
	}
	if err := x.Row(head); err != nil {
		return nil, err
	}
	loc := s.Loc
	if loc == nil {
		loc = time.UTC
	}
	return &xlsxTable{x: x, types: types, places: places, loc: loc}, nil
}

// exportColumnType is the fieldtype a column's cells are typed by: the field's
// own, or the standard column's; "" for anything read as text.
func exportColumnType(d *meta.DocType, col string) string {
	if d == nil {
		return ""
	}
	switch col {
	case "creation":
		return "Datetime"
	case "modified":
		// the exact instant, as text: Data Import compares it to refuse a
		// row changed since the export, and a day number keeps milliseconds
		// at best, on another clock
		return ""
	case "docstatus", "idx":
		return "Int"
	}
	if f := d.Field(col); f != nil {
		return f.Fieldtype
	}
	return ""
}

type xlsxTable struct {
	x     *tabular.XLSXWriter
	types []string
	// places is a Currency column's own precision; nil keeps the site's
	places []*int
	loc    *time.Location
}

func (t *xlsxTable) Write(row []any) error {
	cells := make([]any, len(row))
	for i, v := range row {
		cells[i] = withMoneyPlaces(xlsxCell(v, t.types[i], t.loc), t.places[i])
	}
	return t.x.Row(cells)
}

func (t *xlsxTable) Close() error { return t.x.Close() }

// xlsxCell is v as a typed cell for a column of fieldtype ft. A value that
// does not parse as its type is kept as the text the CSV would hold, rather
// than dropped.
func xlsxCell(v any, ft string, loc *time.Location) any {
	if v == nil {
		return nil
	}
	switch {
	case ft == "Currency":
		if f, ok := exportNumber(v); ok {
			return tabular.XLSXMoney(f)
		}
	case meta.IsNumericType(ft):
		if f, ok := exportNumber(v); ok {
			return f
		}
	case ft == "Check":
		if b, ok := v.(bool); ok {
			return b
		}
	case ft == "Date" || ft == "Month":
		if s, ok := v.(string); ok && len(s) >= 10 {
			if d, err := time.Parse("2006-01-02", s[:10]); err == nil {
				return tabular.XLSXDate{Serial: tabular.TimeToExcelSerial(d), Kind: tabular.DateOnly}
			}
		}
	case ft == "Datetime":
		if s, ok := v.(string); ok && s != "" {
			if at, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return tabular.XLSXDate{Serial: tabular.TimeToExcelSerial(at.In(loc)), Kind: tabular.DateTime}
			}
		}
	case ft == "Time":
		if s, ok := v.(string); ok && s != "" {
			if at, err := time.Parse("15:04:05.999999999", s); err == nil {
				day := time.Duration(at.Hour())*time.Hour + time.Duration(at.Minute())*time.Minute +
					time.Duration(at.Second())*time.Second + time.Duration(at.Nanosecond())
				return tabular.XLSXDate{Serial: float64(day.Milliseconds()) / 86400000, Kind: tabular.TimeOnly}
			}
		}
	}
	switch x := v.(type) {
	case bool, float64, int64:
		if ft == "" { // the attachments table: a size and two flags
			return x
		}
	}
	return csvCell(v)
}

// withMoneyPlaces shows a money cell with the places of its own column — a
// Currency field whose precision differs from the site's, such as one in
// whole units (#159). Any other cell, or no places, is kept as it is.
func withMoneyPlaces(cell any, places *int) any {
	if m, ok := cell.(tabular.XLSXMoney); ok && places != nil {
		return tabular.XLSXMoneyAt{Value: float64(m), Places: *places}
	}
	return cell
}

func exportNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}
