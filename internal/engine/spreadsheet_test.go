package engine

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

// setupSheet is setupPDF's Receipt, on a site in São Paulo so a Datetime
// cell shows its wall clock.
func setupSheet(t *testing.T) *Engine {
	t.Helper()
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, pdfApp())}}, Test: true,
		Lang: "pt-BR", Currency: "BRL", Timezone: "America/Sao_Paulo", DataDir: t.TempDir()})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": "ana@x.com", "full_name": "Ana"})
		if err != nil {
			return err
		}
		u["roles"] = []any{map[string]any{"role": "Gestor"}}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		_, err = c.Insert(Doc{"doctype": "Receipt", "id": "REC-1", "donor": "Carlos Silva", "amount": 250}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func readFileBytes(t *testing.T, e *Engine, url string) []byte {
	t.Helper()
	var b []byte
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		var err error
		b, err = c.ReadAttachment(url)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func zipText(t *testing.T, b []byte, name string) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatal(err)
	}
	f, err := zr.Open(name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	defer f.Close()
	s, _ := io.ReadAll(f)
	return string(s)
}

const gridSheets = `[{
  name: "Grade",
  columns: [
    { label: "Reference", fieldtype: "Data" },
    { label: "Pieces", fieldtype: "Int" },
    { label: "Unit Cost", fieldtype: "Currency" },
    { label: "Delivery Date", fieldtype: "Date" },
    { label: "Approved At", fieldtype: "Datetime" },
    { label: "Paid", fieldtype: "Check" },
    { label: "Free" },
  ],
  rows: [["REF-1", 12, 49.9, "2027-03-15", "2027-03-15T15:30:00Z", 1, 7], ["REF-2", 3]],
}]`

// #152: server code writes a workbook from its own columns and rows, typed
// by fieldtype as the export types its cells, and gets a private File on the
// document back.
func TestSpreadsheetFile_XLSX(t *testing.T) {
	e := setupSheet(t)
	f, err := evalJS(e, "Admin", `ddcore.spreadsheet.file({ doctype: "Receipt", id: "REC-1",
  sheets: [...`+gridSheets+`, { name: "Header", columns: [{ label: "Order" }], rows: [["PO-1"]] }] })`, false)
	if err != nil {
		t.Fatal(err)
	}
	if f["file_name"] != "Receipt-REC-1.xlsx" || f["content_type"] != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" ||
		f["attached_to_doctype"] != "Receipt" || f["attached_to_id"] != "REC-1" {
		t.Fatalf("unexpected File: %v", f)
	}
	if url := db.Str(f["file_url"]); !strings.HasPrefix(url, "/private/files/") {
		t.Fatalf("the workbook must be private, got %s", url)
	}
	b := readFileBytes(t, e, db.Str(f["file_url"]))
	s, err := tabular.Read(b, tabular.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(s.Headers, ",") != "Reference,Pieces,Unit Cost,Delivery Date,Approved At,Paid,Free" || len(s.Rows) != 2 {
		t.Fatalf("headers %v, %d rows", s.Headers, len(s.Rows))
	}
	r := s.Rows[0].Cells
	if r[0].Text != "REF-1" || r[1].Num == nil || *r[1].Num != 12 || r[2].Num == nil || *r[2].Num != 49.9 {
		t.Errorf("text and numbers: %+v", r[:3])
	}
	if r[3].Num == nil || !tabular.ExcelSerialToTime(*r[3].Num, false).Equal(time.Date(2027, 3, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("date: %+v", r[3])
	}
	// 15:30 UTC is 12:30 in São Paulo
	if r[4].Num == nil || !tabular.ExcelSerialToTime(*r[4].Num, false).Equal(time.Date(2027, 3, 15, 12, 30, 0, 0, time.UTC)) {
		t.Errorf("datetime on the site clock: %+v", r[4])
	}
	if r[5].Bool == nil || !*r[5].Bool {
		t.Errorf("a Check's 1 is a boolean: %+v", r[5])
	}
	if r[6].Num == nil || *r[6].Num != 7 {
		t.Errorf("a column with no fieldtype keeps the number: %+v", r[6])
	}
	if got := s.Rows[1].Cells; len(got) > 2 && got[2].Text != "" {
		t.Errorf("a short row is padded with empty cells: %+v", got)
	}
	header := zipText(t, b, "xl/worksheets/sheet2.xml")
	if !strings.Contains(header, "PO-1") {
		t.Errorf("second sheet: %s", header)
	}
	if wb := zipText(t, b, "xl/workbook.xml"); !strings.Contains(wb, `name="Grade"`) || !strings.Contains(wb, `name="Header"`) {
		t.Errorf("sheet names: %s", wb)
	}
	if st := zipText(t, b, "xl/styles.xml"); !strings.Contains(st, `formatCode="#,##0.00"`) {
		t.Errorf("Currency shows the site's decimals: %s", st)
	}
}

// The CSV holds what the export's CSV holds: the BOM, CRLF, the separator
// asked for, a Check as true/false.
func TestSpreadsheetFile_CSV(t *testing.T) {
	e := setupSheet(t)
	f, err := evalJS(e, "Admin", `ddcore.spreadsheet.file({ doctype: "Receipt", id: "REC-1", filename: "grade.csv", sep: ";",
  sheets: `+gridSheets+` })`, false)
	if err != nil {
		t.Fatal(err)
	}
	if f["file_name"] != "grade.csv" || f["content_type"] != "text/csv; charset=utf-8" {
		t.Fatalf("unexpected File: %v", f)
	}
	got := string(readFileBytes(t, e, db.Str(f["file_url"])))
	want := "\ufeffReference;Pieces;Unit Cost;Delivery Date;Approved At;Paid;Free\r\n" +
		"REF-1;12;49.9;2027-03-15;2027-03-15T15:30:00Z;true;7\r\n" +
		"REF-2;3;;;;;\r\n"
	if got != want {
		t.Fatalf("csv:\n%q\nwant\n%q", got, want)
	}

	// format says csv: the extension follows
	f, err = evalJS(e, "Admin", `ddcore.spreadsheet.file({ format: "csv", doctype: "Receipt", id: "REC-1", filename: "grade",
  sheets: [{ columns: [{ label: "a" }], rows: [] }] })`, false)
	if err != nil {
		t.Fatal(err)
	}
	if f["file_name"] != "grade.csv" {
		t.Fatalf("named %v", f["file_name"])
	}
}

func TestSpreadsheetFile_Refusals(t *testing.T) {
	e := setupSheet(t)
	long := `"x".repeat(32768)`
	cases := []struct{ name, opts, typ, msg string }{
		{"no id", `{ doctype: "Receipt", sheets: [{ columns: [], rows: [] }] }`, "ValidationError", "needs a doctype and an id"},
		{"missing document", `{ doctype: "Receipt", id: "REC-9", sheets: [{ columns: [], rows: [] }] }`, "DoesNotExistError", "REC-9"},
		{"no sheets", `{ doctype: "Receipt", id: "REC-1", sheets: [] }`, "ValidationError", "at least one sheet"},
		{"format", `{ format: "ods", doctype: "Receipt", id: "REC-1", sheets: [{ columns: [], rows: [] }] }`, "ValidationError", "ods"},
		{"wrong extension", `{ doctype: "Receipt", id: "REC-1", filename: "a.csv", format: "xlsx", sheets: [{ columns: [], rows: [] }] }`, "ValidationError", "a.csv"},
		{"unknown fieldtype", `{ doctype: "Receipt", id: "REC-1", sheets: [{ columns: [{ label: "a", fieldtype: "Currancy" }], rows: [] }] }`, "ValidationError", "Currancy"},
		{"table fieldtype", `{ doctype: "Receipt", id: "REC-1", sheets: [{ columns: [{ label: "a", fieldtype: "Table" }], rows: [] }] }`, "ValidationError", "Table"},
		{"row too long", `{ doctype: "Receipt", id: "REC-1", sheets: [{ columns: [{ label: "a" }], rows: [[1, 2]] }] }`, "ValidationError", "2"},
		{"cell too long", `{ doctype: "Receipt", id: "REC-1", sheets: [{ name: "G", columns: [{ label: "a" }, { label: "b" }], rows: [["", ` + long + `]] }] }`, "ValidationError", "B2"},
		{"sheet name", `{ doctype: "Receipt", id: "REC-1", sheets: [{ name: "a/b", columns: [], rows: [] }] }`, "ValidationError", "a/b"},
		{"sheet name too long", `{ doctype: "Receipt", id: "REC-1", sheets: [{ name: "x".repeat(32), columns: [], rows: [] }] }`, "ValidationError", ""},
		{"history", `{ doctype: "Receipt", id: "REC-1", sheets: [{ name: "History", columns: [], rows: [] }] }`, "ValidationError", "History"},
		{"duplicate sheets", `{ doctype: "Receipt", id: "REC-1", sheets: [{ name: "G", columns: [], rows: [] }, { name: "g", columns: [], rows: [] }] }`, "ValidationError", "g"},
		{"csv sheets", `{ format: "csv", doctype: "Receipt", id: "REC-1", sheets: [{ columns: [], rows: [] }, { columns: [], rows: [] }] }`, "ValidationError", "2"},
		{"csv sep", `{ format: "csv", sep: ";;", doctype: "Receipt", id: "REC-1", sheets: [{ columns: [], rows: [] }] }`, "ValidationError", ";;"},
		{"xlsx sep", `{ sep: ";", doctype: "Receipt", id: "REC-1", sheets: [{ columns: [], rows: [] }] }`, "ValidationError", "sep"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := evalJS(e, "Admin", `ddcore.spreadsheet.file(`+tc.opts+`)`, false)
			ce := cerr.From(err)
			if ce == nil || ce.Type != tc.typ || !strings.Contains(ce.Message, tc.msg) {
				t.Fatalf("want %s with %q, got %v", tc.typ, tc.msg, err)
			}
		})
	}
	if n := len(pdfFiles(t, e)); n != 0 {
		t.Fatalf("refusals left %d files", n)
	}
	// a long text is fine in a CSV, which has no cell limit
	if _, err := evalJS(e, "Admin", `ddcore.spreadsheet.file({ format: "csv", doctype: "Receipt", id: "REC-1",
  sheets: [{ columns: [{ label: "a" }], rows: [[`+long+`]] }] })`, true); err != nil {
		t.Fatal(err)
	}
}

// Attaching is writing, as for print.pdf.
func TestSpreadsheetFile_AttachingNeedsWrite(t *testing.T) {
	e := setupSheet(t)
	code := `ddcore.spreadsheet.file({ doctype: "Receipt", id: "REC-1", sheets: [{ columns: [{ label: "a" }], rows: [[1]] }]%s })`
	_, err := evalJS(e, "ana@x.com", strings.Replace(code, "%s", "", 1), false)
	if ce := cerr.From(err); ce == nil || ce.Type != "PermissionError" {
		t.Fatalf("a reader attached a spreadsheet: %v", err)
	}
	if n := len(pdfFiles(t, e)); n != 0 {
		t.Fatalf("a refused spreadsheet left %d files", n)
	}
	if _, err := evalJS(e, "ana@x.com", strings.Replace(code, "%s", ", ignorePermissions: true", 1), false); err != nil {
		t.Fatal(err)
	}
	if n := len(pdfFiles(t, e)); n != 1 {
		t.Fatalf("expected 1 file, got %d", n)
	}
}

func TestSpreadsheetFile_RollbackLeavesNoFile(t *testing.T) {
	e := setupSheet(t)
	if _, err := evalJS(e, "Admin", `ddcore.spreadsheet.file({ doctype: "Receipt", id: "REC-1", sheets: `+gridSheets+` })`, true); err != nil {
		t.Fatal(err)
	}
	if n := len(pdfFiles(t, e)); n != 0 {
		t.Fatalf("a rolled back spreadsheet left %d files", n)
	}
}

// The issue's own flow: write the workbook, then mail it in the same call.
func TestSpreadsheetFile_AttachedToAMessage(t *testing.T) {
	e := setupSheet(t)
	out, err := evalJS(e, "Admin", `(() => {
  const xlsx = ddcore.spreadsheet.file({ doctype: "Receipt", id: "REC-1", sheets: `+gridSheets+` });
  const sent = ddcore.sendMail({ template: "demo.aviso", to: "cliente@x.com", args: { pedido: "1", nome: "C", url: "https://x" },
    attach: [xlsx.id], reference: { doctype: "Receipt", id: "REC-1" } });
  return { file: xlsx.id, delivery: sent.delivery };
})()`, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.Select(context.Background(), e.DB.Pool, `SELECT attachments FROM tab_email_delivery WHERE id = $1`, out["delivery"])
	if err != nil || len(rows) != 1 {
		t.Fatalf("delivery %v not found: %v", out["delivery"], err)
	}
	if got := asStrings(rows[0]["attachments"]); len(got) != 1 || got[0] != out["file"] {
		t.Fatalf("the message carries %v, want [%v]", got, out["file"])
	}
}
