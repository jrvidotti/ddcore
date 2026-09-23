package engine

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/tabular"
)

// dataImportFiles plants the DocTypes Data Import is exercised against: a
// Customer with a series id and a title (so a Link can name it by title), and
// a Contact with one field of every kind a cell has to be parsed into, a
// read-only and a secret field, a restricted field, two fields sharing a
// label, and a controller that refuses one sentinel name.
var dataImportFiles = map[string]string{
	"ddcore.app.ts": `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor", "Importador", "SemImport", "SoLeitura"] });`,
	"doctypes/cliente/cliente.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cliente", label: "Customer", idGeneration: { series: "CLI-.####" }, titleField: "nome",
  fields: [{ fieldname: "nome", fieldtype: "Data", label: "Name", reqd: true }],
  permissions: [{ role: "Importador", read: true, write: true, create: true, import: true }] });`,
	"doctypes/contato/contato.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Contato", label: "Contact",
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Name", reqd: true },
    { fieldname: "codigo", fieldtype: "Data", label: "Code", unique: true },
    { fieldname: "cliente", fieldtype: "Link", label: "Customer", options: "Cliente" },
    { fieldname: "tipo", fieldtype: "Select", label: "Kind", options: ["Lead", "Customer"] },
    { fieldname: "ativo", fieldtype: "Check", label: "Active", default: true },
    { fieldname: "nascimento", fieldtype: "Date", label: "Birthday" },
    { fieldname: "visita", fieldtype: "Datetime", label: "Visit" },
    { fieldname: "limite", fieldtype: "Currency", label: "Credit Limit" },
    { fieldname: "qtd", fieldtype: "Int", label: "Quantity" },
    { fieldname: "calculado", fieldtype: "Data", label: "Computed", readOnly: true },
    { fieldname: "senha", fieldtype: "Password", label: "Password" },
    { fieldname: "nota", fieldtype: "Data", label: "Note", permlevel: 1 },
    { fieldname: "same_a", fieldtype: "Data", label: "Same" },
    { fieldname: "same_b", fieldtype: "Data", label: "Same" },
  ],
  permissions: [
    { role: "Importador", read: true, write: true, create: true, import: true },
    { role: "Importador", permlevel: 1, read: true },
    { role: "SemImport", read: true, write: true, create: true },
    { role: "SoLeitura", read: true, import: true },
  ] });`,
	"doctypes/contato/contato.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Contato", {
  validate(doc) { if (doc.nome === "BOOM") ddcore.throw("Refused by the controller"); },
});`,
	"translations/pt-BR.csv": "Credit Limit,Limite de crédito,\nLead,Contato inicial,\n",
}

func setupDataImport(t *testing.T) *Engine {
	t.Helper()
	e := setupWith(t, dataImportFiles)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, u := range []struct{ email, role string }{
			{"imp@x.com", "Importador"}, {"sem@x.com", "SemImport"}, {"leitura@x.com", "SoLeitura"},
		} {
			d, _ := c.NewDoc("User", Doc{"email": u.email, "full_name": u.email})
			d["roles"] = []any{map[string]any{"role": u.role}}
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func runDataImport(t *testing.T, e *Engine, user, lang string, a DataImportArgs) (*DataImportResult, error) {
	t.Helper()
	c := e.NewCtx(context.Background(), user)
	c.Lang = lang
	if a.FileName == "" {
		a.FileName = "rows.csv"
	}
	return c.DataImport(a)
}

func countRows(t *testing.T, e *Engine, table string) int {
	t.Helper()
	var n int
	if err := e.DB.Pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func rowStatuses(res *DataImportResult) string {
	var b []string
	for _, r := range res.Rows {
		s := r.Status
		if r.Message != "" {
			s += ": " + r.Message
		}
		b = append(b, s)
	}
	return strings.Join(b, "\n")
}

const contactsCSV = "Name,Code,Customer,Kind,Active,Birthday,Credit Limit,Quantity,Notes\r\n" +
	"Ana,A1,Acme,Lead,no,31/12/1990,\"1.234,5\",3,first\r\n" +
	"BOOM,B1,,,,,,,\r\n" +
	",C1,,,,,,,missing name\r\n" +
	"Dora,D1,,,maybe,,,,bad check\r\n" +
	"Eva,E1,,,,,,abc,bad number\r\n" +
	"Fábio,F1,CLI-0001,Customer,,,,,\r\n"

func TestDataImportInsertsThroughTheDocumentPath(t *testing.T) {
	e := setupDataImport(t)
	ctx := context.Background()
	if err := e.Run(ctx, "imp@x.com", func(c *Ctx) error {
		d, _ := c.NewDoc("Cliente", Doc{"nome": "Acme"})
		_, err := c.Insert(d, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	a := DataImportArgs{Doctype: "Contato", File: []byte(contactsCSV), Decimal: ",", DateOrder: "dmy"}

	dry := a
	dry.DryRun = true
	before := countRows(t, e, "tab_contato")
	dres, err := runDataImport(t, e, "imp@x.com", "en", dry)
	if err != nil {
		t.Fatal(err)
	}
	if n := countRows(t, e, "tab_contato"); n != before {
		t.Fatalf("a dry run wrote %d rows", n-before)
	}
	if countAudit(t, e, "data.import") != 0 {
		t.Fatal("a dry run must not be audited")
	}

	res, err := runDataImport(t, e, "imp@x.com", "en", a)
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts != (DataImportCounts{Rows: 6, Inserted: 2, Errors: 4}) {
		t.Fatalf("counts %+v\n%s", res.Counts, rowStatuses(res))
	}
	// the dry run predicts the run, row for row
	if rowStatuses(dres) != rowStatuses(res) {
		t.Fatalf("dry run:\n%s\nrun:\n%s", rowStatuses(dres), rowStatuses(res))
	}
	want := []struct {
		line    int
		status  string
		message string
	}{
		{2, "inserted", ""},
		{3, "error", "Refused by the controller"},
		{4, "error", "Name"},
		{5, "error", `Active: "maybe" is not yes or no`},
		{6, "error", `Quantity: "abc" is not a number`},
		{7, "inserted", ""},
	}
	for i, w := range want {
		r := res.Rows[i]
		if r.Row != w.line || r.Status != w.status || !strings.Contains(r.Message, w.message) {
			t.Fatalf("row %d: %+v, want %+v", i, r, w)
		}
		if r.Status == "error" && len(r.Cells) != 9 {
			t.Fatalf("an error row carries its cells: %+v", r)
		}
	}
	if len(res.Rows[0].ID) == 0 || res.Rows[0].Cells != nil {
		t.Fatalf("a good row has an id and no cells: %+v", res.Rows[0])
	}

	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		ana, err := c.GetDoc("Contato", res.Rows[0].ID)
		if err != nil {
			return err
		}
		if ana.Str("cliente") != "CLI-0001" || ana["ativo"] != false || ana.Str("nascimento") != "1990-12-31" ||
			toFloat(ana["limite"]) != 1234.5 || toFloat(ana["qtd"]) != 3 || ana.Str("owner") != "imp@x.com" {
			t.Fatalf("ana: %+v", ana)
		}
		fabio, err := c.GetDoc("Contato", res.Rows[5].ID)
		if err != nil {
			return err
		}
		// a blank Check keeps the field's default on a new record
		if fabio.Str("cliente") != "CLI-0001" || fabio["ativo"] != true || fabio.Str("tipo") != "Customer" {
			t.Fatalf("fabio: %+v", fabio)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if countAudit(t, e, "data.import") != 1 {
		t.Fatal("a real run is audited once")
	}
	// the unknown column is reported, not refused
	last := res.Columns[len(res.Columns)-1]
	if last.Header != "Notes" || last.Status != "unknown" {
		t.Fatalf("columns: %+v", res.Columns)
	}
}

func countAudit(t *testing.T, e *Engine, action string) int {
	t.Helper()
	var n int
	if err := e.DB.Pool.QueryRow(context.Background(), "SELECT count(*) FROM tab_audit_event WHERE action = $1", action).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDataImportDryRunLeavesSeriesAndCatchesDuplicatesInTheFile(t *testing.T) {
	e := setupDataImport(t)
	file := []byte("Name\nAcme\nBeta\n")
	res, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Cliente", File: file, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts.Inserted != 2 || res.Rows[0].ID != "CLI-0001" || res.Rows[1].ID != "CLI-0002" {
		t.Fatalf("%+v", res.Rows)
	}
	// the dry run's series numbers were rolled back with it
	res, err = runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Cliente", File: file})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0].ID != "CLI-0001" {
		t.Fatalf("series advanced by the dry run: %+v", res.Rows)
	}

	dup := []byte("Name,Code\nA,X1\nB,X1\n")
	res, err = runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", File: dup, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if res.Rows[0].Status != "inserted" || res.Rows[1].Status != "error" || res.Rows[1].Type != "DuplicateEntryError" {
		t.Fatalf("%s", rowStatuses(res))
	}
}

func TestDataImportPermissions(t *testing.T) {
	e := setupDataImport(t)
	file := []byte("Name\nA\n")
	for _, tc := range []struct{ user, mode string }{
		{"sem@x.com", "insert"},     // create, but no import
		{"leitura@x.com", "insert"}, // import, but no create
		{"leitura@x.com", "update"}, // import, but no write
		{"Guest", "insert"},
	} {
		_, err := runDataImport(t, e, tc.user, "en", DataImportArgs{Doctype: "Contato", Mode: tc.mode, File: []byte("id,Name\nx,A\n")})
		if cerr.From(err).Type != "PermissionError" {
			t.Fatalf("%s %s: %v", tc.user, tc.mode, err)
		}
	}
	for _, dt := range []string{"Audit Event", "Webhook"} {
		if _, err := runDataImport(t, e, "Admin", "en", DataImportArgs{Doctype: dt, File: file}); cerr.From(err).Type != "ValidationError" {
			t.Fatalf("%s: %v", dt, err)
		}
	}
	if _, err := runDataImport(t, e, "Admin", "en", DataImportArgs{Doctype: "Item Pedido", File: file}); cerr.From(err).Type != "ValidationError" {
		t.Fatalf("child table: %v", err)
	}
	if _, err := runDataImport(t, e, "Admin", "en", DataImportArgs{Doctype: "Contato", Mode: "upsert", File: file}); cerr.From(err).Type != "ValidationError" {
		t.Fatalf("mode: %v", err)
	}
}

func TestDataImportColumns(t *testing.T) {
	e := setupDataImport(t)
	head := "ID,nome,Limite de crédito,Computed,Password,Note,Same,same_b,owner,Name,,Contato inicial\nx,A,1,c,p,n,s,s2,o,A2,,\n"
	res, err := runDataImport(t, e, "imp@x.com", "pt-BR", DataImportArgs{Doctype: "Contato", File: []byte(head), DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, c := range res.Columns {
		got[c.Header] = c.Status + "/" + c.Fieldname
	}
	want := map[string]string{
		"ID":                "mapped/id", // Contato's ids are random: an insert may name one
		"nome":              "mapped/nome",
		"Limite de crédito": "mapped/limite", // the label in the request's language
		"Computed":          "ignored/calculado",
		"Password":          "ignored/senha",
		"Note":              "ignored/nota", // readable at level 1, not writable
		"Same":              "ignored/",
		"same_b":            "mapped/same_b",
		"owner":             "ignored/",
		"Name":              "ignored/nome", // nome already has a column
		"":                  "ignored/",
		"Contato inicial":   "unknown/",
	}
	for h, w := range want {
		if got[h] != w {
			t.Fatalf("%q: got %q, want %q (all: %v)", h, got[h], w, got)
		}
	}
	if res.Counts.Inserted != 1 || res.Rows[0].ID != "x" {
		t.Fatalf("%s", rowStatuses(res))
	}

	// an override maps an unknown header, and "" drops a matched one
	res, err = runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", File: []byte("Full name,Code\nZé,Z1\n"), DryRun: true,
		Columns: map[string]string{"Full name": "nome", "Code": ""}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Columns[0].Status != "mapped" || res.Columns[1].Status != "ignored" || res.Counts.Inserted != 1 {
		t.Fatalf("%+v %s", res.Columns, rowStatuses(res))
	}

	// a series DocType takes no id from the file
	res, err = runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Cliente", File: []byte("id,Name\nMINE,Acme\n")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Columns[0].Status != "ignored" || res.Rows[0].ID != "CLI-0001" {
		t.Fatalf("%+v %+v", res.Columns, res.Rows)
	}

	if _, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", File: []byte("Foo,Bar\n1,2\n")}); cerr.From(err).Type != "ValidationError" {
		t.Fatalf("no matching column: %v", err)
	}
	if _, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", Mode: "update", File: []byte("Name\nA\n")}); cerr.From(err).Type != "ValidationError" {
		t.Fatalf("update without id: %v", err)
	}
	if _, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", File: []byte("Name\nA\nB\nC\n"), MaxRows: 2}); cerr.From(err).Type != "ValidationError" {
		t.Fatalf("row cap: %v", err)
	}
}

func TestDataImportUpdate(t *testing.T) {
	e := setupDataImport(t)
	ctx := context.Background()
	var modified string
	err := e.Run(ctx, "imp@x.com", func(c *Ctx) error {
		for _, v := range []Doc{{"id": "c1", "nome": "Ana", "codigo": "A", "qtd": 1}, {"id": "c2", "nome": "Bia", "codigo": "B", "qtd": 2}} {
			d, _ := c.NewDoc("Contato", v)
			saved, err := c.Insert(d, SaveOpts{})
			if err != nil {
				return err
			}
			modified = saved.Str("modified")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	file := "id,Quantity,Code,modified\n" +
		"c1,10,,\n" + // a blank cell clears the field
		"c2,20,B,2001-01-01T00:00:00Z\n" + // changed since that export
		"nope,1,,\n" +
		",1,,\n"
	res, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", Mode: "update", File: []byte(file)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts != (DataImportCounts{Rows: 4, Updated: 1, Errors: 3}) {
		t.Fatalf("%+v\n%s", res.Counts, rowStatuses(res))
	}
	if res.Rows[1].Type != "TimestampMismatchError" || res.Rows[2].Type != "DoesNotExistError" || res.Rows[3].Type != "MandatoryError" {
		t.Fatalf("%s", rowStatuses(res))
	}
	_ = modified
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		c1, err := c.GetDoc("Contato", "c1")
		if err != nil {
			return err
		}
		if toFloat(c1["qtd"]) != 10 || c1["codigo"] != nil || c1.Str("nome") != "Ana" {
			t.Fatalf("c1: %+v", c1)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// An export read back as an update changes nothing and fails nothing: the
// columns an export carries and a Data Import accepts are the same shape.
func TestDataImportExportRoundTrip(t *testing.T) {
	e := setupDataImport(t)
	ctx := context.Background()
	err := e.Run(ctx, "imp@x.com", func(c *Ctx) error {
		for _, v := range []Doc{
			{"nome": "Ana", "codigo": "A", "qtd": 1, "limite": 10.5, "nascimento": "1990-01-02", "visita": "2024-05-06 07:08:09", "tipo": "Lead", "ativo": false},
			{"nome": "Bia", "codigo": "B"},
		} {
			d, _ := c.NewDoc("Contato", v)
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		sink := &CSVSink{Open: func(string) (io.Writer, error) { return &buf, nil }, Sep: ';',
			Lookup: func(n string) (*meta.DocType, error) { return c.St.DocType(n) }}
		_, err := c.Export(ExportArgs{Doctype: "Contato"}, sink)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := runDataImport(t, e, "imp@x.com", "en", DataImportArgs{Doctype: "Contato", Mode: "update", File: buf.Bytes()})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts != (DataImportCounts{Rows: 2, Updated: 2}) {
		t.Fatalf("%+v\n%s\n%s", res.Counts, rowStatuses(res), buf.String())
	}
}

func TestDataImportSubmittedDocuments(t *testing.T) {
	e := setupDataImport(t)
	ctx := context.Background()
	var id string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Cli"})
		if _, err := c.Insert(p, SaveOpts{}); err != nil {
			return err
		}
		d, _ := c.NewDoc("Pedido", Doc{"cliente": "Cli"})
		saved, err := c.Insert(d, SaveOpts{})
		if err != nil {
			return err
		}
		saved["docstatus"] = 1
		saved, err = c.Save(saved, SaveOpts{})
		id = saved.ID()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	file := "id,Obs,Desconto\n" + id + ",later note,\n" + id + ",,5\n"
	res, err := runDataImport(t, e, "Admin", "en", DataImportArgs{Doctype: "Pedido", Mode: "update", File: []byte(file)})
	if err != nil {
		t.Fatal(err)
	}
	// an allow-on-submit field changes; any other is refused
	if res.Rows[0].Status != "updated" || res.Rows[1].Status != "error" {
		t.Fatalf("%s", rowStatuses(res))
	}
}

func TestParseCell(t *testing.T) {
	pt := map[string]string{"Yes": "Sim", "No": "Não", "Lead": "Contato inicial"}
	tr := func(s string, a ...any) string {
		if v, ok := pt[s]; ok {
			return v
		}
		return s
	}
	f := func(ft string) *meta.Field { return &meta.Field{Fieldname: "x", Fieldtype: ft, Label: "X"} }
	num := func(n float64) tabular.Cell { return tabular.Cell{Text: "n", Num: &n} }
	txt := func(s string) tabular.Cell { return tabular.Cell{Text: s} }
	br := cellConv{decimal: ",", order: "dmy"}
	us := cellConv{decimal: ".", order: "mdy"}
	sel := &meta.Field{Fieldname: "x", Fieldtype: "Select", Label: "X", Options: []any{"Lead", "Customer"}}
	ok := []struct {
		f    *meta.Field
		cell tabular.Cell
		conv cellConv
		want any
	}{
		{f("Currency"), txt("1.234,56"), br, 1234.56},
		{f("Currency"), txt("1,234.56"), us, 1234.56},
		{f("Currency"), txt(" 12 "), us, 12.0},
		{f("Percent"), txt("12,5%"), br, 12.5},
		{f("Int"), txt("1.000"), br, int64(1000)},
		{f("Int"), num(7), us, int64(7)},
		{f("Check"), txt("Sim"), br, true},
		{f("Check"), txt("FALSE"), br, false},
		{f("Check"), num(1), br, true},
		{f("Check"), txt(""), br, nil},
		{f("Date"), txt("31/12/1990"), br, "1990-12-31"},
		{f("Date"), txt("12/31/1990"), us, "1990-12-31"},
		{f("Date"), txt("1990-12-31"), us, "1990-12-31"},
		{f("Date"), txt("5.1.24"), br, "2024-01-05"},
		{f("Date"), num(45292), br, "2024-01-01"},
		{f("Datetime"), txt("31/12/1990 08:05"), br, "1990-12-31 08:05:00"},
		{f("Datetime"), txt("2024-01-01T10:00:00Z"), br, "2024-01-01T10:00:00Z"},
		{f("Datetime"), num(45292.5), br, "2024-01-01 12:00:00"},
		{f("Time"), num(0.75), br, "18:00:00"},
		{sel, txt("contato inicial"), br, "Lead"},
		{sel, txt("customer"), br, "Customer"},
		{sel, txt("Other"), br, "Other"},
		{f("Data"), txt(" 007 "), br, "007"},
		{f("Data"), txt(""), br, nil},
	}
	for _, c := range ok {
		got, err := parseCell(c.f, "X", c.cell, c.conv, tr)
		if err != nil || got != c.want {
			t.Fatalf("%s %q: got %#v (%v), want %#v", c.f.Fieldtype, c.cell.Text, got, err, c.want)
		}
	}
	bad := []struct {
		ft   string
		text string
		conv cellConv
	}{
		{"Currency", "abc", br}, {"Int", "1,5", br}, {"Float", "1e400", us}, {"Check", "maybe", br},
		{"Date", "31/02/2024", br}, {"Date", "yesterday", br}, {"Datetime", "31/12/1990 25:00", br},
	}
	for _, c := range bad {
		if _, err := parseCell(f(c.ft), "X", txt(c.text), c.conv, tr); cerr.From(err).Type != "ValidationError" {
			t.Fatalf("%s %q: want a validation error, got %v", c.ft, c.text, err)
		}
	}
}

func TestDataImportTemplate(t *testing.T) {
	e := setupDataImport(t)
	err := e.Run(context.Background(), "imp@x.com", func(c *Ctx) error {
		c.Lang = "pt-BR"
		h, err := c.DataImportTemplate("Contato")
		if err != nil {
			return err
		}
		got := strings.Join(h, "|")
		// no read-only, secret or unwritable field; a shared label becomes
		// the fieldname; labels come in the request's language
		if got != "id|Nome|Código|Customer|Kind|Active|Birthday|Visit|Limite de crédito|Quantity|same_a|same_b" {
			t.Fatalf("template: %s", got)
		}
		c.Lang = "en"
		h, _ = c.DataImportTemplate("Cliente")
		if strings.Join(h, "|") != "Name" {
			t.Fatalf("series template: %v", h)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(context.Background(), "sem@x.com", func(c *Ctx) error {
		_, err := c.DataImportTemplate("Contato")
		return err
	})
	if cerr.From(err).Type != "PermissionError" {
		t.Fatalf("template without import: %v", err)
	}
}
