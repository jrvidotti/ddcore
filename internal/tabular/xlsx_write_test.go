package tabular

import (
	"archive/zip"
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func writeWorkbook(t *testing.T, o XLSXOptions, rows ...[]any) []byte {
	t.Helper()
	var buf bytes.Buffer
	x, err := NewXLSXWriter(&buf, o)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if err := x.Row(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func zipPart(t *testing.T, b []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, _ := f.Open()
			defer rc.Close()
			s, _ := io.ReadAll(rc)
			return string(s)
		}
	}
	t.Fatalf("no %s in the workbook", name)
	return ""
}

// What the writer puts out, the Data Import reader reads back: the same
// header, text, numbers, booleans and day numbers.
func TestXLSXWriterRoundTrips(t *testing.T) {
	when := time.Date(2026, 1, 31, 14, 30, 15, 0, time.UTC)
	b := writeWorkbook(t, XLSXOptions{SheetName: "Portal Payment"},
		[]any{"id", "customer_name", "total", "paid", "paid_at", "note"},
		[]any{"PAY-1", `Endereço <"A&B">`, XLSXMoney(1234.5), true, XLSXDate{TimeToExcelSerial(when), DateTime}, nil},
		[]any{"PAY-2", "x\x01y", int64(7), false, XLSXDate{TimeToExcelSerial(when), DateOnly}, ""},
	)
	s, err := Read(b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Format != "xlsx" || strings.Join(s.Headers, ",") != "id,customer_name,total,paid,paid_at,note" {
		t.Fatalf("headers: %v (%s)", s.Headers, s.Format)
	}
	if len(s.Rows) != 2 {
		t.Fatalf("rows: %d", len(s.Rows))
	}
	r := s.Rows[0].Cells
	if r[1].Text != `Endereço <"A&B">` {
		t.Errorf("text: %q", r[1].Text)
	}
	if r[2].Num == nil || *r[2].Num != 1234.5 {
		t.Errorf("money is a number: %+v", r[2])
	}
	if r[3].Bool == nil || !*r[3].Bool {
		t.Errorf("check is a boolean: %+v", r[3])
	}
	if r[4].Num == nil || !ExcelSerialToTime(*r[4].Num, false).Equal(when) {
		t.Errorf("datetime: %+v", r[4])
	}
	if got := s.Rows[1].Cells[1].Text; got != "x\uFFFDy" {
		t.Errorf("a control character XML cannot hold: %q", got)
	}
	if n := s.Rows[1].Cells[2].Num; n == nil || *n != 7 {
		t.Errorf("int: %+v", s.Rows[1].Cells[2])
	}
}

// Numbers carry a style only where a reader needs one: dates show as dates
// and money with its decimals; the header is bold.
func TestXLSXWriterStylesCells(t *testing.T) {
	b := writeWorkbook(t, XLSXOptions{MoneyDecimals: 0},
		[]any{"a", "b", "c", "d"},
		[]any{XLSXDate{46000, DateOnly}, XLSXDate{46000.5, DateTime}, XLSXDate{0.25, TimeOnly}, XLSXMoney(10)},
	)
	sheet := zipPart(t, b, "xl/worksheets/sheet1.xml")
	for _, want := range []string{
		`<c r="A1" s="1" t="inlineStr">`,
		`<c r="A2" s="2"><v>46000</v></c>`,
		`<c r="B2" s="3"><v>46000.5</v></c>`,
		`<c r="C2" s="4"><v>0.25</v></c>`,
		`<c r="D2" s="5"><v>10</v></c>`,
		`state="frozen"`,
	} {
		if !strings.Contains(sheet, want) {
			t.Errorf("sheet lacks %s:\n%s", want, sheet)
		}
	}
	if styles := zipPart(t, b, "xl/styles.xml"); !strings.Contains(styles, `formatCode="#,##0"`) {
		t.Errorf("a currency without minor units has no decimals: %s", styles)
	}
	if styles := zipPart(t, writeWorkbook(t, XLSXOptions{MoneyDecimals: 2}, []any{"a"}), "xl/styles.xml"); !strings.Contains(styles, `formatCode="#,##0.00"`) {
		t.Errorf("two decimals: %s", styles)
	}
}

func TestXLSXSheetNameFollowsExcelRules(t *testing.T) {
	for in, want := range map[string]string{
		"":                                      "Sheet1",
		"Students: [2026]":                      "Students   2026",
		"A very long DocType name that goes on": "A very long DocType name that g",
	} {
		if got := SheetName(in); got != want {
			t.Errorf("SheetName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTimeToExcelSerialIsTheWallClock(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	at := time.Date(2026, 3, 1, 9, 0, 0, 0, loc)
	// 2026-03-01 is day 46082; 09:00 is 0.375 of it, in the zone the time is in
	if got := TimeToExcelSerial(at); got != 46082.375 {
		t.Fatalf("serial = %v", got)
	}
	if got := ColumnName(26); got != "AA" {
		t.Fatalf("ColumnName(26) = %s", got)
	}
}

// A workbook of several sheets: each is listed in the workbook, has its own
// part and its own bold header, and the reader still takes the first.
func TestXLSXWriterWritesSeveralSheets(t *testing.T) {
	var buf bytes.Buffer
	x, err := NewXLSXWriter(&buf, XLSXOptions{Sheets: []string{"Header", "Grid"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range [][]any{{"order"}, {"PO-1"}} {
		if err := x.Row(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.NextSheet(); err != nil {
		t.Fatal(err)
	}
	for _, r := range [][]any{{"ref", "pieces"}, {"REF-1", int64(12)}} {
		if err := x.Row(r); err != nil {
			t.Fatal(err)
		}
	}
	if err := x.NextSheet(); err != ErrNoMoreSheets {
		t.Fatalf("past the last sheet: %v", err)
	}
	if err := x.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	wb := zipPart(t, b, "xl/workbook.xml")
	if !strings.Contains(wb, `<sheet name="Header" sheetId="1" r:id="rId1"/><sheet name="Grid" sheetId="2" r:id="rId2"/>`) {
		t.Errorf("workbook: %s", wb)
	}
	rels := zipPart(t, b, "xl/_rels/workbook.xml.rels")
	if !strings.Contains(rels, `Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet2.xml"`) ||
		!strings.Contains(rels, `Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles"`) {
		t.Errorf("rels: %s", rels)
	}
	if ct := zipPart(t, b, "[Content_Types].xml"); !strings.Contains(ct, `/xl/worksheets/sheet2.xml`) {
		t.Errorf("content types: %s", ct)
	}
	grid := zipPart(t, b, "xl/worksheets/sheet2.xml")
	if !strings.Contains(grid, `<c r="A1" s="1" t="inlineStr"><is><t xml:space="preserve">ref</t>`) ||
		!strings.Contains(grid, `<c r="B2"><v>12</v></c>`) || strings.Contains(grid, "PO-1") {
		t.Errorf("second sheet: %s", grid)
	}
	s, err := Read(b, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Headers, ",") != "order" || len(s.Rows) != 1 || s.Rows[0].Cells[0].Text != "PO-1" {
		t.Errorf("the reader takes the first sheet: %v %+v", s.Headers, s.Rows)
	}
}

// Excel will not open two sheets of one name, whatever their case.
func TestXLSXWriterRefusesDuplicateSheets(t *testing.T) {
	if _, err := NewXLSXWriter(io.Discard, XLSXOptions{Sheets: []string{"Grid", "grid"}}); err == nil {
		t.Fatal("two sheets named grid were accepted")
	}
	if _, err := NewXLSXWriter(io.Discard, XLSXOptions{Sheets: []string{"a/b", "a b"}}); err == nil {
		t.Fatal("two names equal once trimmed were accepted")
	}
}

// A sheet the caller never reached is still in the workbook, empty.
func TestXLSXWriterFillsSheetsNeverReached(t *testing.T) {
	b := writeWorkbook(t, XLSXOptions{Sheets: []string{"One", "Two", "Three"}}, []any{"h"})
	for _, n := range []string{"sheet2", "sheet3"} {
		if p := zipPart(t, b, "xl/worksheets/"+n+".xml"); !strings.Contains(p, "<sheetData></sheetData></worksheet>") {
			t.Errorf("%s: %s", n, p)
		}
	}
}
