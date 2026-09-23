// Package tabular reads the spreadsheet a person hands to Data Import: a CSV
// in whatever shape Excel or LibreOffice saved it, or the first sheet of an
// XLSX workbook. It knows nothing about DocTypes; it turns bytes into a header
// and rows of cells, and keeps each row's line number so an error can point at
// the row the person sees in their spreadsheet.
package tabular

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Cell is one value. Text is always set; Num is set too when the XLSX stored
// the cell as a number, and Bool when it stored a boolean — a date in a
// workbook is a number with a date style, and only the field it lands in can
// say which one it is.
type Cell struct {
	Text string
	Num  *float64
	Bool *bool
}

// Row is one data row, with the number the spreadsheet shows next to it
// (the header is row 1).
type Row struct {
	Line  int
	Cells []Cell
}

// Sheet is what was read.
type Sheet struct {
	Format   string // "csv" | "xlsx"
	Sep      rune   // the CSV separator used; 0 for an XLSX
	Date1904 bool   // the workbook counts days from 1904 (old Mac Excel)
	Headers  []string
	Rows     []Row
}

// Options tunes a read.
type Options struct {
	Sep     rune // CSV separator; 0 sniffs it from the header line
	MaxRows int  // data rows allowed; 0 means no limit
}

// TooManyRowsError is returned as soon as the file has more data rows than
// Options.MaxRows allows, without reading the rest.
type TooManyRowsError struct{ Max int }

func (e *TooManyRowsError) Error() string {
	return fmt.Sprintf("the file has more than %d rows", e.Max)
}

// ErrEmpty is a file with no header row.
var ErrEmpty = errors.New("the file is empty")

// ErrLegacyXLS is an Excel 97–2003 workbook, which this reader does not parse.
var ErrLegacyXLS = errors.New("legacy .xls workbooks are not supported: save it as .xlsx or CSV")

var (
	zipMagic = []byte("PK\x03\x04")
	oleMagic = []byte{0xD0, 0xCF, 0x11, 0xE0}
)

// Read parses b as an XLSX workbook when it is a zip archive, and as CSV
// otherwise. The file name is not trusted to say which: a browser reports
// whatever extension the person gave it.
func Read(b []byte, o Options) (*Sheet, error) {
	var (
		s   *Sheet
		err error
	)
	switch {
	case bytes.HasPrefix(b, zipMagic):
		s, err = readXLSX(b, o)
	case bytes.HasPrefix(b, oleMagic):
		return nil, ErrLegacyXLS
	default:
		s, err = readCSV(b, o)
	}
	if err != nil {
		return nil, err
	}
	if len(s.Headers) == 0 {
		return nil, ErrEmpty
	}
	for i, h := range s.Headers {
		s.Headers[i] = strings.TrimSpace(h)
	}
	return s, nil
}

// collect appends a data row unless every cell is blank — spreadsheets leave
// formatted but empty rows at the bottom — and enforces the row cap.
func (s *Sheet) collect(line int, cells []Cell, max int) error {
	blank := true
	for _, c := range cells {
		if strings.TrimSpace(c.Text) != "" {
			blank = false
			break
		}
	}
	if blank {
		return nil
	}
	if max > 0 && len(s.Rows) >= max {
		return &TooManyRowsError{Max: max}
	}
	s.Rows = append(s.Rows, Row{Line: line, Cells: cells})
	return nil
}

// Cell returns the i-th cell of r, or an empty one past its end: rows in both
// formats may be shorter than the header.
func (r Row) Cell(i int) Cell {
	if i < 0 || i >= len(r.Cells) {
		return Cell{}
	}
	return r.Cells[i]
}

// Texts is the row as plain strings, padded to n cells.
func (r Row) Texts(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = r.Cell(i).Text
	}
	return out
}

// ExcelSerialToTime converts a workbook's day number to a civil time. The
// 1900 system counts from 1899-12-30 rather than 1900-01-01 because Lotus
// believed 1900 was a leap year and Excel kept the bug; every date from
// 1900-03-01 on comes out right from that epoch.
func ExcelSerialToTime(serial float64, date1904 bool) time.Time {
	epoch := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	if date1904 {
		epoch = time.Date(1904, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	ms := int64(serial*86400000 + 0.5)
	if serial < 0 {
		ms = int64(serial*86400000 - 0.5)
	}
	return epoch.Add(time.Duration(ms) * time.Millisecond)
}

// formatNum is how a numeric XLSX cell reads as text: the shortest form that
// round-trips, so 0.1 stored as 0.10000000000000001 reads "0.1" and a code
// stored as the number 123 reads "123".
func formatNum(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}
