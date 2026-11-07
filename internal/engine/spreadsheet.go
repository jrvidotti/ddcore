package engine

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

// SpreadsheetArgs is ddcore.spreadsheet.file's argument.
type SpreadsheetArgs struct {
	Format            string             `json:"format"` // "xlsx" or "csv"; "" reads the filename's extension, else xlsx
	Doctype           string             `json:"doctype"`
	ID                string             `json:"id"`
	Filename          string             `json:"filename"`
	Sep               string             `json:"sep"` // CSV only; "" is a comma
	Sheets            []SpreadsheetSheet `json:"sheets"`
	IgnorePermissions bool               `json:"ignorePermissions"`
}

// SpreadsheetSheet is one sheet: a header of columns and the rows under it,
// each row a list of values in the columns' order.
type SpreadsheetSheet struct {
	Name    string              `json:"name"`
	Columns []SpreadsheetColumn `json:"columns"`
	Rows    [][]any             `json:"rows"`
}

// SpreadsheetColumn is a header cell and the fieldtype its cells are typed by
// ("" keeps each value's own JSON type).
type SpreadsheetColumn struct {
	Label     string `json:"label"`
	Fieldtype string `json:"fieldtype"`
}

const (
	xlsxContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	csvContentType  = "text/csv; charset=utf-8"
	// xlsxMaxCell is the longest text an Excel cell holds, in UTF-16 units.
	xlsxMaxCell = 32767
)

// SpreadsheetFile writes columns and rows to an XLSX workbook or a CSV and
// stores it as a private File attached to a document: how server code gets a
// spreadsheet to mail, as PrintPDF gets a PDF. The cells follow the export's
// rules (xlsxCell, csvCell), so the file reads like the desk's own export.
// Attaching needs write on the document, as an upload does, unless
// IgnorePermissions. The File is written on the caller's transaction, so a
// rollback takes it and its bytes back out.
func (c *Ctx) SpreadsheetFile(a SpreadsheetArgs) (Doc, error) {
	if c.User == "Guest" && !a.IgnorePermissions {
		return nil, cerr.Auth("Sign in to upload files")
	}
	if a.Doctype == "" || a.ID == "" {
		return nil, cerr.Validation("spreadsheet.file needs a doctype and an id")
	}
	format, name, err := spreadsheetName(a)
	if err != nil {
		return nil, err
	}
	sep, err := spreadsheetSep(format, a.Sep)
	if err != nil {
		return nil, err
	}
	sheets, err := spreadsheetSheets(format, a.Sheets)
	if err != nil {
		return nil, err
	}
	var doc Doc
	run := func() error {
		id, err := c.UploadTarget(a.Doctype, a.ID, "")
		if err != nil {
			return err
		}
		if id == "" {
			// UploadTarget lets a file wait, detached, for a document not
			// saved yet; this one names its owner, which must exist
			return cerr.NotFound("{0} {1} not found", a.Doctype, a.ID)
		}
		var buf bytes.Buffer
		ct := csvContentType
		if format == "xlsx" {
			ct = xlsxContentType
			err = writeSpreadsheetXLSX(&buf, sheets, c.E.Location(), c.E.CurrencyPrecision())
		} else {
			err = writeSpreadsheetCSV(&buf, sheets[0], sep)
		}
		if err != nil {
			return err
		}
		doc, err = c.StoreFile(NewFile{Doctype: a.Doctype, ID: id, Name: name, ContentType: ct,
			Size: int64(buf.Len()), Private: true}, bytes.NewReader(buf.Bytes()))
		return err
	}
	if a.IgnorePermissions {
		err = c.WithIgnorePermissions(run)
	} else {
		err = run()
	}
	return doc, err
}

// spreadsheetName settles the format and the file name: a name without the
// format's extension gets it, one ending in the other format's is refused.
func spreadsheetName(a SpreadsheetArgs) (format, name string, err error) {
	ext := strings.ToLower(filepath.Ext(a.Filename))
	format = strings.ToLower(a.Format)
	if format == "" {
		format = "xlsx"
		if ext == ".csv" {
			format = "csv"
		}
	}
	if format != "xlsx" && format != "csv" {
		return "", "", cerr.Validation("Spreadsheet format {0} is not supported: use xlsx or csv", a.Format)
	}
	name = a.Filename
	if name == "" {
		name = a.Doctype + "-" + a.ID
	}
	switch ext {
	case "." + format:
	case ".xlsx", ".xls", ".csv":
		return "", "", cerr.Validation("File name {0} does not end in .{1}", a.Filename, format)
	default:
		name += "." + format
	}
	return format, name, nil
}

func spreadsheetSep(format, sep string) (rune, error) {
	if sep == "" {
		return 0, nil
	}
	if format != "csv" {
		return 0, cerr.Validation("sep applies to a CSV only")
	}
	rs := []rune(sep)
	if len(rs) != 1 || rs[0] == '"' || rs[0] == '\n' || rs[0] == '\r' {
		return 0, cerr.Validation("Invalid CSV separator: {0}", sep)
	}
	return rs[0], nil
}

// spreadsheetSheets checks the sheets before anything is written: their
// names and columns, the shape of every row and, for a workbook, Excel's
// limits — which the export's writer meets by trimming, and an app asked for
// exact cells should hear about instead. A Check column's 0 or 1 becomes a
// boolean, which is what an app's JS tends to hold.
func spreadsheetSheets(format string, in []SpreadsheetSheet) ([]SpreadsheetSheet, error) {
	if len(in) == 0 {
		return nil, cerr.Validation("spreadsheet.file needs at least one sheet")
	}
	if format == "csv" && len(in) > 1 {
		return nil, cerr.Validation("A CSV holds one sheet; {0} were given", len(in))
	}
	seen := map[string]bool{}
	out := make([]SpreadsheetSheet, len(in))
	for si, sh := range in {
		if sh.Name == "" {
			sh.Name = fmt.Sprintf("Sheet%d", si+1)
		}
		if format == "xlsx" {
			if !validSheetName(sh.Name) {
				return nil, cerr.Validation("Sheet name {0} is not allowed: at most 31 characters, none of [ ] : * ? / \\, no apostrophe or space at either end, and not History", sh.Name)
			}
			if seen[strings.ToLower(sh.Name)] {
				return nil, cerr.Validation("Two sheets are named {0}", sh.Name)
			}
			seen[strings.ToLower(sh.Name)] = true
			if len(sh.Rows) >= tabular.MaxXLSXRows {
				return nil, cerr.Validation("Sheet {0} has {1} rows; an XLSX sheet holds at most {2}", sh.Name, len(sh.Rows), tabular.MaxXLSXRows-1)
			}
		}
		for ci, col := range sh.Columns {
			if col.Fieldtype != "" && meta.ColumnType(col.Fieldtype) == "" {
				return nil, cerr.Validation("Column {0} of sheet {1}: {2} is not a fieldtype that holds a value", ci+1, sh.Name, col.Fieldtype)
			}
			if format == "xlsx" && utf16Len(col.Label) > xlsxMaxCell {
				return nil, cerr.Validation("Cell {0} of sheet {1} holds {2} characters; an XLSX cell holds at most {3}", tabular.ColumnName(ci)+"1", sh.Name, utf16Len(col.Label), xlsxMaxCell)
			}
		}
		rows := make([][]any, len(sh.Rows))
		for ri, row := range sh.Rows {
			if len(row) > len(sh.Columns) {
				return nil, cerr.Validation("Row {0} of sheet {1} has {2} cells, but the sheet has {3} columns", ri+1, sh.Name, len(row), len(sh.Columns))
			}
			cells := make([]any, len(sh.Columns))
			copy(cells, row)
			for ci, v := range cells {
				if sh.Columns[ci].Fieldtype == "Check" {
					if f, ok := v.(float64); ok && (f == 0 || f == 1) {
						cells[ci] = f == 1
					}
				}
				if s, ok := cells[ci].(string); ok && format == "xlsx" && len(s) > xlsxMaxCell && utf16Len(s) > xlsxMaxCell {
					return nil, cerr.Validation("Cell {0} of sheet {1} holds {2} characters; an XLSX cell holds at most {3}", tabular.ColumnName(ci)+fmt.Sprint(ri+2), sh.Name, utf16Len(s), xlsxMaxCell)
				}
			}
			rows[ri] = cells
		}
		sh.Rows = rows
		out[si] = sh
	}
	return out, nil
}

// validSheetName is a name Excel takes as it is: tabular.SheetName would not
// change it, and neither apostrophe-wrapped nor History, which Excel keeps.
func validSheetName(n string) bool {
	return tabular.SheetName(n) == n && !strings.HasPrefix(n, "'") && !strings.HasSuffix(n, "'") &&
		!strings.EqualFold(n, "History")
}

// utf16Len is the length Excel counts a text in.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n++
		if r > 0xFFFF {
			n++
		}
	}
	return n
}

func writeSpreadsheetXLSX(w io.Writer, sheets []SpreadsheetSheet, loc *time.Location, moneyDecimals int) error {
	names := make([]string, len(sheets))
	for i, sh := range sheets {
		names[i] = sh.Name
	}
	x, err := tabular.NewXLSXWriter(w, tabular.XLSXOptions{Sheets: names, MoneyDecimals: moneyDecimals})
	if err != nil {
		return err
	}
	for i, sh := range sheets {
		if i > 0 {
			if err := x.NextSheet(); err != nil {
				return err
			}
		}
		head := make([]any, len(sh.Columns))
		for ci, col := range sh.Columns {
			head[ci] = col.Label
		}
		if err := x.Row(head); err != nil {
			return err
		}
		for _, row := range sh.Rows {
			cells := make([]any, len(row))
			for ci, v := range row {
				cells[ci] = xlsxCell(v, sh.Columns[ci].Fieldtype, loc)
			}
			if err := x.Row(cells); err != nil {
				return err
			}
		}
	}
	return x.Close()
}

func writeSpreadsheetCSV(w io.Writer, sh SpreadsheetSheet, sep rune) error {
	cw, err := newCSVWriter(w, sep)
	if err != nil {
		return err
	}
	head := make([]string, len(sh.Columns))
	for ci, col := range sh.Columns {
		head[ci] = col.Label
	}
	if err := cw.Write(head); err != nil {
		return err
	}
	for _, row := range sh.Rows {
		cells := make([]string, len(row))
		for ci, v := range row {
			cells[ci] = csvCell(v)
		}
		if err := cw.Write(cells); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}
