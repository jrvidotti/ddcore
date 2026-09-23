package tabular

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/encoding/charmap"
)

func texts(s *Sheet) [][]string {
	var out [][]string
	for _, r := range s.Rows {
		out = append(out, r.Texts(len(s.Headers)))
	}
	return out
}

func eq(t *testing.T, got, want any) {
	t.Helper()
	if g, w := fmt.Sprintf("%q", got), fmt.Sprintf("%q", want); g != w {
		t.Fatalf("got  %s\nwant %s", g, w)
	}
}

func TestCSVBOMAndComma(t *testing.T) {
	s, err := Read([]byte("\xef\xbb\xbfCode,Title\r\nP-1,\"Hello, world\"\r\nP-2,\"say \"\"hi\"\"\"\r\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, s.Format, "csv")
	eq(t, s.Headers, []string{"Code", "Title"})
	eq(t, texts(s), [][]string{{"P-1", "Hello, world"}, {"P-2", `say "hi"`}})
	if s.Rows[0].Line != 2 || s.Rows[1].Line != 3 {
		t.Fatalf("lines: %d %d", s.Rows[0].Line, s.Rows[1].Line)
	}
}

func TestCSVSniffsSemicolonAndTab(t *testing.T) {
	s, err := Read([]byte("Code;Amount;Note\nP-1;1.234,56;\"a;b\"\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Sep != ';' {
		t.Fatalf("sep %q", s.Sep)
	}
	eq(t, texts(s), [][]string{{"P-1", "1.234,56", "a;b"}})

	s, err = Read([]byte("Code\tTitle, with comma\tNote\nP-1\tx\ty\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.Sep != '\t' {
		t.Fatalf("sep %q", s.Sep)
	}
	// an explicit separator wins over the sniff
	s, err = Read([]byte("a;b,c\n1;2,3\n"), Options{Sep: ','})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, s.Headers, []string{"a;b", "c"})
}

func TestCSVMultilineKeepsLineNumbersAndDropsBlankRows(t *testing.T) {
	s, err := Read([]byte("Code,Note\nP-1,\"line one\nline two\"\n,\nP-2,x\n\n,,\n"), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rows) != 2 {
		t.Fatalf("rows: %v", texts(s))
	}
	if s.Rows[0].Line != 2 || s.Rows[1].Line != 5 {
		t.Fatalf("lines: %d %d", s.Rows[0].Line, s.Rows[1].Line)
	}
	eq(t, s.Rows[0].Cells[1].Text, "line one\nline two")
}

func TestCSVWindows1252(t *testing.T) {
	enc, err := charmap.Windows1252.NewEncoder().String("Nome;Endereço\nJoão;Rua São José\n")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Read([]byte(enc), Options{})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, s.Headers, []string{"Nome", "Endereço"})
	eq(t, texts(s), [][]string{{"João", "Rua São José"}})
}

func TestCSVRowCap(t *testing.T) {
	_, err := Read([]byte("a\n1\n2\n3\n"), Options{MaxRows: 2})
	var tm *TooManyRowsError
	if !errors.As(err, &tm) || tm.Max != 2 {
		t.Fatalf("want TooManyRowsError, got %v", err)
	}
	if _, err := Read([]byte("a\n1\n2\n"), Options{MaxRows: 2}); err != nil {
		t.Fatal(err)
	}
}

func TestEmpty(t *testing.T) {
	if _, err := Read([]byte(""), Options{}); !errors.Is(err, ErrEmpty) {
		t.Fatalf("got %v", err)
	}
	if _, err := Read([]byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1}, Options{}); !errors.Is(err, ErrLegacyXLS) {
		t.Fatalf("got %v", err)
	}
}

// ---------------------------------------------------------------- xlsx

type part struct{ name, body string }

func buildZip(t *testing.T, parts ...part) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

const nsMain = `xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"`

func workbook(t *testing.T, date1904 bool, sheet string, shared string) []byte {
	pr := ""
	if date1904 {
		pr = `<workbookPr date1904="1"/>`
	}
	parts := []part{
		{"[Content_Types].xml", `<Types/>`},
		{"xl/workbook.xml", `<workbook ` + nsMain + `>` + pr + `<sheets><sheet name="Data" sheetId="1" r:id="rId7"/><sheet name="Other" sheetId="2" r:id="rId8"/></sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId8" Target="worksheets/sheet2.xml"/><Relationship Id="rId7" Target="/xl/worksheets/data.xml"/></Relationships>`},
		{"xl/worksheets/data.xml", `<worksheet ` + nsMain + `><sheetData>` + sheet + `</sheetData></worksheet>`},
		{"xl/worksheets/sheet2.xml", `<worksheet ` + nsMain + `><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>wrong sheet</t></is></c></row></sheetData></worksheet>`},
	}
	if shared != "" {
		parts = append(parts, part{"xl/sharedStrings.xml", `<sst ` + nsMain + `>` + shared + `</sst>`})
	}
	return buildZip(t, parts...)
}

func TestXLSXFirstSheetCells(t *testing.T) {
	shared := `<si><t>Code</t></si><si><t>Title</t></si><si><r><t>Bold </t></r><r><t>and plain</t></r><rPh><t>ignored</t></rPh></si><si><t xml:space="preserve"> P-1 </t></si>`
	sheet := `<row r="1"><c r="A1" t="s"><v>0</v></c><c r="B1" t="s"><v>1</v></c><c r="D1" t="inlineStr"><is><t>Done</t></is></c><c r="E1" t="inlineStr"><is><t>Due</t></is></c></row>` +
		`<row r="2"><c r="A2" t="s"><v>3</v></c><c r="B2" t="s"><v>2</v></c><c r="D2" t="b"><v>1</v></c><c r="E2"><v>45292</v></c></row>` +
		`<row r="4"><c r="B4" t="str"><f>CONCAT("a","b")</f><v>ab</v></c><c r="C4"><v>0.10000000000000001</v></c><c r="D4" t="b"><v>0</v></c></row>` +
		`<row r="5"><c r="A5"/></row>`
	s, err := Read(workbook(t, false, sheet, shared), Options{})
	if err != nil {
		t.Fatal(err)
	}
	eq(t, s.Format, "xlsx")
	eq(t, s.Headers, []string{"Code", "Title", "", "Done", "Due"})
	eq(t, texts(s), [][]string{{" P-1 ", "Bold and plain", "", "true", "45292"}, {"", "ab", "0.1", "false", ""}})
	if s.Rows[0].Line != 2 || s.Rows[1].Line != 4 {
		t.Fatalf("lines %d %d", s.Rows[0].Line, s.Rows[1].Line)
	}
	if n := s.Rows[0].Cells[4].Num; n == nil || *n != 45292 {
		t.Fatalf("num: %v", n)
	}
	if b := s.Rows[0].Cells[3].Bool; b == nil || !*b {
		t.Fatalf("bool: %v", b)
	}
	if s.Date1904 {
		t.Fatal("1900 system expected")
	}
}

func TestXLSXDate1904AndSerial(t *testing.T) {
	s, err := Read(workbook(t, true, `<row><c t="inlineStr"><is><t>d</t></is></c></row><row><c><v>1</v></c></row>`, ""), Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !s.Date1904 || len(s.Rows) != 1 {
		t.Fatalf("%+v", s)
	}
	if got := ExcelSerialToTime(45292, false).Format("2006-01-02"); got != "2024-01-01" {
		t.Fatal(got)
	}
	if got := ExcelSerialToTime(0, true).Format("2006-01-02"); got != "1904-01-01" {
		t.Fatal(got)
	}
	if got := ExcelSerialToTime(45292.75, false); !got.Equal(time.Date(2024, 1, 1, 18, 0, 0, 0, time.UTC)) {
		t.Fatal(got)
	}
}

func TestXLSXErrors(t *testing.T) {
	notWorkbook := buildZip(t, part{"hello.txt", "hi"})
	if _, err := Read(notWorkbook, Options{}); !errors.Is(err, errNotWorkbook) {
		t.Fatalf("got %v", err)
	}
	many := `<row><c t="inlineStr"><is><t>h</t></is></c></row>` + strings.Repeat(`<row><c><v>1</v></c></row>`, 3)
	var tm *TooManyRowsError
	if _, err := Read(workbook(t, false, many, ""), Options{MaxRows: 2}); !errors.As(err, &tm) {
		t.Fatalf("got %v", err)
	}
	bad := `<row><c r="A1" t="s"><v>9</v></c></row>`
	if _, err := Read(workbook(t, false, bad, ""), Options{}); err == nil {
		t.Fatal("unknown shared string should fail")
	}
	far := `<row><c r="ZZZZ1"><v>1</v></c></row>`
	if _, err := Read(workbook(t, false, far, ""), Options{}); err == nil {
		t.Fatal("a column past XFD should fail")
	}
}

func TestXLSXEntrySizeCap(t *testing.T) {
	// a highly compressible sheet just above the cap
	big := `<row><c t="inlineStr"><is><t>` + strings.Repeat("a", maxEntryBytes) + `</t></is></c></row>`
	if _, err := Read(workbook(t, false, big, ""), Options{}); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("got %v", err)
	}
}

func TestColumnIndex(t *testing.T) {
	for ref, want := range map[string]int{"A1": 0, "Z9": 25, "AA1": 26, "xfd3": 16383} {
		got, err := columnIndex(ref)
		if err != nil || got != want {
			t.Fatalf("%s: %d %v", ref, got, err)
		}
	}
	if _, err := columnIndex("12"); err == nil {
		t.Fatal("want error")
	}
}
