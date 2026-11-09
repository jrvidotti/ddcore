package tabular

// A deliberately small XLSX writer: worksheets streamed row by row, so an
// export of a hundred thousand rows holds one row in memory and not the sheet.
//
// The parts that do not depend on the rows go into the archive first, which is
// why every sheet is named up front; the worksheets follow one after the other,
// each left open while its rows arrive and closed by NextSheet or by Close,
// which also writes the zip's central directory. Strings are inline
// (`t="inlineStr"`), which avoids a shared-strings table that could only be
// written after every row had been seen.
//
// Cells are typed, which is the whole point over a CSV: a number is a number
// whatever the reader's locale, a boolean is TRUE/FALSE, and a date is a day
// number with a date format — Excel shows it in the reader's own date order.

import (
	"archive/zip"
	"bufio"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// MaxXLSXRows is Excel's own row limit, the header included.
const MaxXLSXRows = 1048576

// maxCellChars is the longest text Excel keeps in a cell; a longer one makes
// it offer to "repair" the workbook.
const maxCellChars = 32767

// DateKind says how an XLSXDate is shown.
type DateKind int

const (
	DateOnly DateKind = iota // a civil date
	DateTime                 // a wall-clock date and time
	TimeOnly                 // a time of day, with no date
)

// XLSXDate is a cell holding a date, a date-time or a time of day as Excel
// keeps them: a serial day number (see TimeToExcelSerial) with a date format.
type XLSXDate struct {
	Serial float64
	Kind   DateKind
}

// XLSXMoney is a number shown with the site's currency decimals and a
// thousands separator.
type XLSXMoney float64

// XLSXMoneyAt is a money cell with places of its own — a Currency field
// whose `precision` differs from the site's, such as one kept in whole
// units. Places outside 0 to 9 are clamped, as a field's precision is.
type XLSXMoneyAt struct {
	Value  float64
	Places int
}

// The cell styles in styles.xml, by index.
const (
	styleHeader   = 1
	styleDate     = 2
	styleDateTime = 3
	styleTime     = 4
	styleMoney    = 5
	// styleMoneyAt+p is a money cell shown with p places, 0 to 9
	styleMoneyAt = 6
	maxMoneyAt   = 9
)

// ErrTooManyXLSXRows is a sheet past Excel's last row.
var ErrTooManyXLSXRows = errors.New("an XLSX sheet holds at most 1048575 data rows; export as CSV or NDJSON instead")

// XLSXWriter writes one workbook. Row writes a sheet's header first and then
// its data, NextSheet moves to the next sheet; Close finishes the file, and
// without it the archive is unreadable.
type XLSXWriter struct {
	zw     *zip.Writer
	w      *bufio.Writer
	rows   int
	sheet  int // the 0-based sheet being written
	sheets int
}

// XLSXOptions shape the workbook.
type XLSXOptions struct {
	SheetName string // trimmed to Excel's rules; "Sheet1" when empty
	// Sheets names every sheet of the workbook, in order, each trimmed as
	// SheetName is; when set, SheetName is ignored. Two names that match
	// once trimmed, ignoring case, are refused: Excel will not open the file.
	Sheets []string
	// MoneyDecimals is how many decimals an XLSXMoney cell shows.
	MoneyDecimals int
}

// ErrNoMoreSheets is NextSheet past the last sheet named in XLSXOptions.
var ErrNoMoreSheets = errors.New("the workbook has no more sheets")

// NewXLSXWriter starts a workbook on w, on its first sheet.
func NewXLSXWriter(w io.Writer, o XLSXOptions) (*XLSXWriter, error) {
	names := o.Sheets
	if len(names) == 0 {
		names = []string{o.SheetName}
	}
	seen := map[string]bool{}
	clean := make([]string, len(names))
	for i, n := range names {
		clean[i] = SheetName(n)
		if seen[strings.ToLower(clean[i])] {
			return nil, fmt.Errorf("the workbook has two sheets named %q", clean[i])
		}
		seen[strings.ToLower(clean[i])] = true
	}
	zw := zip.NewWriter(w)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", xlsxContentTypes(len(clean))},
		{"_rels/.rels", xlsxRootRels},
		{"xl/workbook.xml", xlsxWorkbook(clean)},
		{"xl/_rels/workbook.xml.rels", xlsxWorkbookRels(len(clean))},
		{"xl/styles.xml", xlsxStylesFor(o.MoneyDecimals)},
	}
	for _, p := range parts {
		f, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := io.WriteString(f, p.body); err != nil {
			return nil, err
		}
	}
	x := &XLSXWriter{zw: zw, sheets: len(clean)}
	return x, x.openSheet()
}

// openSheet starts the worksheet x.sheet.
func (x *XLSXWriter) openSheet() error {
	f, err := x.zw.Create(fmt.Sprintf("xl/worksheets/sheet%d.xml", x.sheet+1))
	if err != nil {
		return err
	}
	x.w = bufio.NewWriterSize(f, 32<<10)
	x.rows = 0
	// the header row stays in view while the rows scroll
	_, err = x.w.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/></sheetView></sheetViews>` +
		`<sheetData>`)
	return err
}

// closeSheet ends the worksheet being written.
func (x *XLSXWriter) closeSheet() error {
	if _, err := x.w.WriteString(`</sheetData></worksheet>`); err != nil {
		return err
	}
	return x.w.Flush()
}

// NextSheet ends the current sheet and starts the next one named in
// XLSXOptions.Sheets, whose first Row is its own header.
func (x *XLSXWriter) NextSheet() error {
	if x.sheet+1 >= x.sheets {
		return ErrNoMoreSheets
	}
	if err := x.closeSheet(); err != nil {
		return err
	}
	x.sheet++
	return x.openSheet()
}

// Row writes the next row. The first one is the header and is set in bold.
// A cell is nil (empty), a string, a bool, a number, an XLSXDate or an
// XLSXMoney; anything else is written as its text.
func (x *XLSXWriter) Row(cells []any) error {
	if x.rows >= MaxXLSXRows {
		return ErrTooManyXLSXRows
	}
	x.rows++
	r := strconv.Itoa(x.rows)
	x.w.WriteString(`<row r="` + r + `">`)
	for i, v := range cells {
		style := 0
		if x.rows == 1 {
			style = styleHeader
		}
		writeCell(x.w, ColumnName(i)+r, v, style)
	}
	_, err := x.w.WriteString(`</row>`)
	return err
}

// Close ends the sheet and the archive; a sheet named but never reached is
// written empty, since the workbook already lists it. It does not close the
// underlying writer.
func (x *XLSXWriter) Close() error {
	for {
		err := x.NextSheet()
		if err == ErrNoMoreSheets {
			break
		}
		if err != nil {
			return err
		}
	}
	if err := x.closeSheet(); err != nil {
		return err
	}
	return x.zw.Close()
}

func writeCell(w *bufio.Writer, ref string, v any, style int) {
	num := func(f float64, style int) {
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return
		}
		fmtc := byte('f')
		if math.Abs(f) >= 1e15 {
			fmtc = 'g'
		}
		w.WriteString(`<c r="` + ref + `"` + styleAttr(style) + `><v>` + strconv.FormatFloat(f, fmtc, -1, 64) + `</v></c>`)
	}
	switch x := v.(type) {
	case nil:
	case string:
		if x == "" {
			return
		}
		if len(x) > maxCellChars && utf8.RuneCountInString(x) > maxCellChars {
			x = string([]rune(x)[:maxCellChars])
		}
		w.WriteString(`<c r="` + ref + `"` + styleAttr(style) + ` t="inlineStr"><is><t xml:space="preserve">`)
		w.WriteString(xmlEscape(x))
		w.WriteString(`</t></is></c>`)
	case bool:
		b := "0"
		if x {
			b = "1"
		}
		w.WriteString(`<c r="` + ref + `"` + styleAttr(style) + ` t="b"><v>` + b + `</v></c>`)
	case float64:
		num(x, style)
	case float32:
		num(float64(x), style)
	case int64:
		num(float64(x), style)
	case int:
		num(float64(x), style)
	case int32:
		num(float64(x), style)
	case XLSXMoney:
		if style == 0 {
			style = styleMoney
		}
		num(float64(x), style)
	case XLSXMoneyAt:
		if style == 0 {
			style = styleMoneyAt + min(max(x.Places, 0), maxMoneyAt)
		}
		num(x.Value, style)
	case XLSXDate:
		if style == 0 {
			style = map[DateKind]int{DateOnly: styleDate, DateTime: styleDateTime, TimeOnly: styleTime}[x.Kind]
		}
		num(x.Serial, style)
	default:
		writeCell(w, ref, fmt.Sprint(v), style)
	}
}

func styleAttr(style int) string {
	if style == 0 {
		return ""
	}
	return ` s="` + strconv.Itoa(style) + `"`
}

// xmlEscape escapes text for an XML element or attribute; a character XML 1.0
// cannot hold (most control characters, invalid UTF-8) becomes U+FFFD.
func xmlEscape(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// ColumnName is the letter name of a 0-based column: "A", …, "Z", "AA".
func ColumnName(i int) string {
	s := ""
	for n := i + 1; n > 0; n = (n - 1) / 26 {
		s = string(rune('A'+(n-1)%26)) + s
	}
	return s
}

// SheetName trims a name to Excel's rules: at most 31 characters, none of
// []:*?/\, and not blank.
func SheetName(name string) string {
	name = strings.TrimSpace(strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, name))
	if r := []rune(name); len(r) > 31 {
		name = strings.TrimSpace(string(r[:31]))
	}
	if name == "" {
		return "Sheet1"
	}
	return name
}

// TimeToExcelSerial is the day number Excel keeps for t's wall clock — the
// date and time t shows in its own location, which is the one the caller put
// it in. The inverse of ExcelSerialToTime for the 1900 system.
func TimeToExcelSerial(t time.Time) float64 {
	wall := time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return float64(wall.Sub(epoch).Milliseconds()) / 86400000
}

// moneyFormat is the number format of a money cell: grouped thousands and
// `decimals` places. Excel swaps the separators for the reader's locale.
func moneyFormat(decimals int) string {
	if decimals <= 0 {
		return "#,##0"
	}
	return "#,##0." + strings.Repeat("0", min(decimals, 15))
}

func xlsxContentTypes(sheets int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>`)
	for i := 1; i <= sheets; i++ {
		fmt.Fprintf(&b, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, i)
	}
	b.WriteString(`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>` +
		`</Types>`)
	return b.String()
}

const xlsxRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

// xlsxWorkbook lists the sheets; sheet i is relationship rId<i>.
func xlsxWorkbook(names []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
		`<sheets>`)
	for i, n := range names {
		fmt.Fprintf(&b, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlEscape(n), i+1, i+1)
	}
	b.WriteString(`</sheets></workbook>`)
	return b.String()
}

// xlsxWorkbookRels points rId1…rIdN at the sheets and rIdN+1 at the styles.
func xlsxWorkbookRels(sheets int) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i := 1; i <= sheets; i++ {
		fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, i, i)
	}
	fmt.Fprintf(&b, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`, sheets+1)
	b.WriteString(`</Relationships>`)
	return b.String()
}

// xlsxStylesFor is the stylesheet: the site's money format as numFmt 164, and
// one format per places 0 to 9 (numFmt 165+p) for an XLSXMoneyAt. The styles
// are written before the first row, so every places a cell may ask for is
// declared up front.
func xlsxStylesFor(moneyDecimals int) string {
	var fmts, xfs strings.Builder
	fmt.Fprintf(&fmts, `<numFmt numFmtId="164" formatCode="%s"/>`, xmlEscape(moneyFormat(moneyDecimals)))
	for p := 0; p <= maxMoneyAt; p++ {
		fmt.Fprintf(&fmts, `<numFmt numFmtId="%d" formatCode="%s"/>`, 165+p, xmlEscape(moneyFormat(p)))
		fmt.Fprintf(&xfs, `<xf numFmtId="%d" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>`, 165+p)
	}
	return fmt.Sprintf(xlsxStyles, maxMoneyAt+2, fmts.String(), styleMoneyAt+maxMoneyAt+1, xfs.String())
}

// The built-in formats 14, 22 and 21 are the locale's short date, date and
// time, and time with seconds: Excel renders them in the reader's own order,
// so a pt-BR reader sees 31/01/2026 and an en-US one 1/31/2026.
const xlsxStyles = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
	`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
	`<numFmts count="%d">%s</numFmts>` +
	`<fonts count="2"><font><sz val="11"/><name val="Calibri"/></font><font><b/><sz val="11"/><name val="Calibri"/></font></fonts>` +
	`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
	`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
	`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
	`<cellXfs count="%d">` +
	`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` +
	`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` +
	`<xf numFmtId="14" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="22" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="21" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` +
	`%s</cellXfs></styleSheet>`
