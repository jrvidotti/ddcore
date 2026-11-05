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
