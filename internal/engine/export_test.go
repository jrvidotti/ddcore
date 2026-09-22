package engine

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// exportApp: a parent with a child table and a Password field, plus three
// roles that differ only in what they may do with an export — the whole point
// of DAT-02 is that those differences survive the export.
func exportApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Exportador", "Leitor", "Dono"] });`)
	w("doctypes/item_nota/item_nota.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Item Nota", isChild: true, fields: [
  { fieldname: "descricao", fieldtype: "Data", label: "Description" },
  { fieldname: "qtd", fieldtype: "Int", label: "Quantity" } ] });`)
	w("doctypes/nota/nota.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Nota", idGeneration: { field: "titulo" },
  fields: [
    { fieldname: "titulo", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "valor", fieldtype: "Currency", label: "Amount" },
    { fieldname: "ativo", fieldtype: "Check", label: "Active", default: true },
    { fieldname: "segredo", fieldtype: "Password", label: "Secret" },
    { fieldname: "itens", fieldtype: "Table", label: "Items", options: "Item Nota" },
  ],
  permissions: [
    { role: "Exportador", read: true, write: true, create: true, delete: true, export: true },
    { role: "Leitor", read: true },
    { role: "Dono", read: true, export: true, ifOwner: true },
  ] });`)
	return dir
}

func setupExport(t *testing.T) *Engine {
	t.Helper()
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres indisponível em DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres indisponível: %v", err)
	}
	e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	e0.DB.Close()
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "demo", Dir: exportApp(t)}}, Test: true, DataDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, u := range []struct{ email, role string }{
			{"exp@x.com", "Exportador"}, {"leitor@x.com", "Leitor"},
			{"ana@x.com", "Dono"}, {"ze@x.com", "Dono"},
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

// collector is an ExportSink that keeps everything, for assertions.
type collector struct {
	columns []string
	docs    []Doc
	files   [][]ExportFile
	sum     *ExportSummary
	ended   bool
}

func (c *collector) Begin(_ *meta.DocType, columns []string) error {
	c.columns = columns
	return nil
}

func (c *collector) Doc(d Doc, f []ExportFile) error {
	c.docs = append(c.docs, d.Clone())
	c.files = append(c.files, f)
	return nil
}

func (c *collector) End(s *ExportSummary) error {
	c.sum, c.ended = s, true
	return nil
}

func (c *collector) names() []string {
	out := make([]string, len(c.docs))
	for i, d := range c.docs {
		out[i] = d.ID()
	}
	return out
}

// makeNotas inserts n notes owned by `user`, titled with a fixed width so the
// keyset order over `name` is the same as the insertion order.
func makeNotas(t *testing.T, e *Engine, user string, prefix string, n int) {
	t.Helper()
	err := e.Run(context.Background(), user, func(c *Ctx) error {
		return c.WithIgnorePermissions(func() error {
			for i := 0; i < n; i++ {
				d, _ := c.NewDoc("Nota", Doc{"titulo": fmt.Sprintf("%s-%04d", prefix, i), "valor": float64(i)})
				if _, err := c.Insert(d, SaveOpts{IgnorePermissions: true}); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		t.Fatal(err)
	}
}

func exportAs(t *testing.T, e *Engine, user string, a ExportArgs) (*collector, error) {
	t.Helper()
	col := &collector{}
	var sum *ExportSummary
	err := e.Run(context.Background(), user, func(c *Ctx) error {
		var e2 error
		sum, e2 = c.Export(a, col)
		return e2
	})
	if err == nil && sum == nil {
		t.Fatal("export without summary and without error")
	}
	return col, err
}

// The walk has to cross page boundaries without skipping or repeating a row —
// the failure an OFFSET walk makes and the reason for the keyset.
func TestExportWalksEveryPage(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 250)

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Batch: 40})
	if err != nil {
		t.Fatal(err)
	}
	if col.sum.Rows != 250 || len(col.docs) != 250 {
		t.Fatalf("expected 250 rows, got %d (sink: %d)", col.sum.Rows, len(col.docs))
	}
	seen := map[string]bool{}
	prev := ""
	for _, n := range col.names() {
		if seen[n] {
			t.Fatalf("duplicate row: %s", n)
		}
		if n <= prev {
			t.Fatalf("out of order: %s after %s", n, prev)
		}
		seen[n], prev = true, n
	}
	if !col.ended || col.sum.Truncated {
		t.Fatalf("unexpected summary: ended=%v truncated=%v", col.ended, col.sum.Truncated)
	}
}

// A page boundary that falls exactly on the last row must not produce a
// phantom extra page, nor stop one row early.
func TestExportHandlesExactPageBoundary(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 80)

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Batch: 40})
	if err != nil {
		t.Fatal(err)
	}
	if col.sum.Rows != 80 {
		t.Fatalf("expected 80, got %d", col.sum.Rows)
	}
}

// deletingSink removes an already-exported document partway through the walk,
// shrinking the set under it. This is what separates a keyset walk from an
// OFFSET one: OFFSET counts from the start of a set that just lost a row, so
// page 2 begins one row late and a document is never exported. The keyset
// resumes from the last name it actually saw and cannot skip.
type deletingSink struct {
	collector
	c     *Ctx
	after int
	done  bool
}

func (s *deletingSink) Doc(d Doc, f []ExportFile) error {
	if err := s.collector.Doc(d, f); err != nil {
		return err
	}
	if !s.done && len(s.collector.docs) == s.after {
		s.done = true
		return s.c.Delete("Nota", s.collector.docs[0].ID(), false, true)
	}
	return nil
}

func TestExportDoesNotSkipWhenTheSetShrinks(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 100)

	var sink *deletingSink
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		sink = &deletingSink{c: c, after: 10}
		_, err := c.Export(ExportArgs{Doctype: "Nota", Batch: 10}, sink)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sink.done {
		t.Fatal("the test did not delete anything")
	}
	seen := map[string]bool{}
	for _, n := range sink.names() {
		seen[n] = true
	}
	for i := 0; i < 100; i++ {
		if name := fmt.Sprintf("N-%04d", i); !seen[name] {
			t.Fatalf("%s was not exported: walk skipped a row when the set shrank", name)
		}
	}
}

func TestExportNestsChildrenInIdxOrder(t *testing.T) {
	e := setupExport(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": "Com itens"})
		d["itens"] = []any{
			map[string]any{"descricao": "a", "qtd": 1},
			map[string]any{"descricao": "b", "qtd": 2},
			map[string]any{"descricao": "c", "qtd": 3},
		}
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Children: true})
	if err != nil {
		t.Fatal(err)
	}
	itens := col.docs[0].Children("itens")
	if len(itens) != 3 {
		t.Fatalf("expected 3 children, got %d", len(itens))
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := itens[i].Str("descricao"); got != want {
			t.Errorf("child %d: expected %q, got %q", i, want, got)
		}
		if itens[i].Str("parent") != col.docs[0].ID() {
			t.Errorf("child %d lost its link to parent", i)
		}
	}
	if col.sum.ChildRows["Item Nota"] != 3 {
		t.Errorf("manifest: expected 3 child rows, got %d", col.sum.ChildRows["Item Nota"])
	}

	// Without Children the field must not appear at all — a consumer has to be
	// able to tell "no children" from "children not exported".
	plain, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := plain.docs[0]["itens"]; ok {
		t.Error("without Children the table field should not be present")
	}
}

// A document with no child rows still declares the field, so every exported
// document has the same shape.
func TestExportEmptyChildTableIsAnEmptyList(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 1)

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Children: true})
	if err != nil {
		t.Fatal(err)
	}
	v, ok := col.docs[0]["itens"]
	if !ok {
		t.Fatal("table field should exist")
	}
	if list, _ := v.([]any); len(list) != 0 {
		t.Fatalf("expected empty list, got %v", v)
	}
}

// ifOwner is the row filter the desk list already applies; the export is not
// a way around it.
func TestExportHonoursIfOwner(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "ana@x.com", "A", 30)
	makeNotas(t, e, "ze@x.com", "Z", 12)

	col, err := exportAs(t, e, "ana@x.com", ExportArgs{Doctype: "Nota", Batch: 7})
	if err != nil {
		t.Fatal(err)
	}
	if col.sum.Rows != 30 {
		t.Fatalf("expected ana's 30 notas, got %d", col.sum.Rows)
	}
	for _, d := range col.docs {
		if d.Str("owner") != "ana@x.com" {
			t.Fatalf("leaked document belonging to %s", d.Str("owner"))
		}
	}
}

// read alone is not enough: taking the rows out as a file is its own
// permission, and until now nothing on the server checked it.
func TestExportRequiresExportPermission(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 3)

	_, err := exportAs(t, e, "leitor@x.com", ExportArgs{Doctype: "Nota"})
	if err == nil {
		t.Fatal("expected rejection for read-only user")
	}
	if got := cerr.From(err).Type; got != "PermissionError" {
		t.Fatalf("expected PermissionError, got %s (%v)", got, err)
	}
}

// A secret must never be in a file that travels.
func TestExportOmitsPasswordFields(t *testing.T) {
	e := setupExport(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": "Com segredo", "segredo": "hunter2"})
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range col.columns {
		if c == "segredo" {
			t.Fatal("Password field included in export columns")
		}
	}
	if _, ok := col.docs[0]["segredo"]; ok {
		t.Fatal("Password field included in exported document")
	}

	// nor by asking for it explicitly
	if _, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Fields: []string{"id", "segredo"}}); err == nil {
		t.Fatal("requesting Password field explicitly should be rejected")
	}
}

// The core's own credential columns are not exportable either.
func TestExportOmitsCredentialColumns(t *testing.T) {
	e := setupExport(t)
	cols := ExportColumns(e.Meta.DocTypes["User"])
	for _, c := range cols {
		if c == "password_hash" || c == "new_password" {
			t.Fatalf("credential column in User export: %s", c)
		}
	}
	if !contains(cols, "email") {
		t.Fatal("User export should include email")
	}
}

func TestExportRefusesAChildDoctype(t *testing.T) {
	e := setupExport(t)
	_, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Item Nota"})
	if err == nil || cerr.From(err).Type != "ValidationError" {
		t.Fatalf("expected ValidationError for child table, got %v", err)
	}
}

// A capped export is a sample, and says so — reconciling one as if it were the
// whole set is the mistake the flag exists to prevent.
func TestExportLimitMarksTruncated(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 50)

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Limit: 20, Batch: 7})
	if err != nil {
		t.Fatal(err)
	}
	if col.sum.Rows != 20 {
		t.Fatalf("expected 20, got %d", col.sum.Rows)
	}
	if !col.sum.Truncated {
		t.Error("export truncated by limit should be marked as truncated")
	}

	// A limit larger than the set is not a truncation.
	full, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Limit: 500})
	if err != nil {
		t.Fatal(err)
	}
	if full.sum.Rows != 50 || full.sum.Truncated {
		t.Errorf("limit above set size: rows=%d truncated=%v", full.sum.Rows, full.sum.Truncated)
	}

	// Nor is a limit that lands exactly on the last row. A flag raised over a
	// complete export is worse than no flag: it teaches the reader to ignore it.
	exact, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Limit: 50, Batch: 10})
	if err != nil {
		t.Fatal(err)
	}
	if exact.sum.Rows != 50 {
		t.Fatalf("expected 50, got %d", exact.sum.Rows)
	}
	if exact.sum.Truncated {
		t.Error("limit matched entire set: nothing was truncated")
	}
}

func TestExportAppliesFilters(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "A", 10)
	makeNotas(t, e, "Admin", "B", 5)

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{
		Doctype: "Nota",
		Filters: []any{[]any{"titulo", "like", "A-%"}},
		Batch:   3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if col.sum.Rows != 10 {
		t.Fatalf("expected 10 A notas, got %d", col.sum.Rows)
	}
	for _, d := range col.docs {
		if !strings.HasPrefix(d.ID(), "A-") {
			t.Fatalf("filter let through %s", d.ID())
		}
	}
}

// The attachment manifest is what a reconciliation compares: the row, its
// checksum, and an honest flag when the bytes are gone.
func TestExportManifestsAttachments(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 1)

	body := []byte("conteúdo do anexo")
	dir := filepath.Join(e.Cfg.DataDir, "files", "private")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "abc.txt"), body, 0o644); err != nil {
		t.Fatal(err)
	}

	var nota string
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Nota", ListArgs{Fields: []string{"id"}})
		if err != nil {
			return err
		}
		nota = rows[0]["id"].(string)
		for _, f := range []Doc{
			{"file_name": "anexo.txt", "file_url": "/private/files/abc.txt", "file_size": len(body),
				"content_type": "text/plain", "is_private": true,
				"attached_to_doctype": "Nota", "attached_to_id": nota, "attached_to_field": "anexo"},
			{"file_name": "sumido.txt", "file_url": "/private/files/nao-existe.txt", "file_size": 10,
				"is_private": true, "attached_to_doctype": "Nota", "attached_to_id": nota},
		} {
			d, _ := c.NewDoc("File", f)
			if _, err := c.Insert(d, SaveOpts{IgnorePermissions: true}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	col, err := exportAs(t, e, "exp@x.com", ExportArgs{Doctype: "Nota", Attachments: true})
	if err != nil {
		t.Fatal(err)
	}
	files := col.files[0]
	if len(files) != 2 {
		t.Fatalf("expected 2 attachments, got %d", len(files))
	}
	sum := sha256.Sum256(body)
	var found, missing bool
	for _, f := range files {
		if f.FileName == "anexo.txt" {
			found = true
			if f.SHA256 != hex.EncodeToString(sum[:]) {
				t.Errorf("wrong checksum: %s", f.SHA256)
			}
			if f.Size != int64(len(body)) {
				t.Errorf("wrong size: %d", f.Size)
			}
			if f.Missing {
				t.Error("attachment exists and was marked missing")
			}
		}
		if f.FileName == "sumido.txt" {
			missing = true
			if !f.Missing {
				t.Error("attachment with no bytes on disk should be marked missing")
			}
		}
	}
	if !found || !missing {
		t.Fatalf("incomplete manifest: found=%v missing=%v", found, missing)
	}
	if col.sum.Files != 2 {
		t.Errorf("attachment count: %d", col.sum.Files)
	}
}

// The attachment of a document you may read belongs in your export even when
// someone else uploaded it — the rule /private/files already applies.
func TestExportCarriesAttachmentsOwnedByOthers(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "ana@x.com", "A", 1)

	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Nota", ListArgs{Fields: []string{"id"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		d, _ := c.NewDoc("File", Doc{"file_name": "de-outro.txt", "file_url": "/private/files/x.txt",
			"is_private": true, "attached_to_doctype": "Nota", "attached_to_id": rows[0]["id"]})
		_, err = c.Insert(d, SaveOpts{IgnorePermissions: true})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	col, err := exportAs(t, e, "ana@x.com", ExportArgs{Doctype: "Nota", Attachments: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(col.files[0]) != 1 {
		t.Fatalf("Admin attachment missing from ana's export: %v", col.files[0])
	}
}

// ---------------------------------------------------------------- sinks

// A CSV cell and the NDJSON field for the same value must not disagree: the
// two files are compared side by side during a reconciliation.
func TestExportSinksAgreeOnValues(t *testing.T) {
	e := setupExport(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": `Rua "do Meio", 3`, "valor": 10.5, "ativo": false})
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var nd, csvBuf bytes.Buffer
	err = e.Run(context.Background(), "exp@x.com", func(c *Ctx) error {
		if _, err := c.Export(ExportArgs{Doctype: "Nota"}, NewNDJSONSink(&nd)); err != nil {
			return err
		}
		sink := &CSVSink{
			Open:   func(string) (io.Writer, error) { return &csvBuf, nil },
			Lookup: func(n string) (*meta.DocType, error) { return c.St.DocType(n) },
		}
		_, err := c.Export(ExportArgs{Doctype: "Nota"}, sink)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	var doc map[string]any
	if err := json.Unmarshal([]byte(strings.SplitN(nd.String(), "\n", 2)[0]), &doc); err != nil {
		t.Fatal(err)
	}
	if doc["valor"] != 10.5 || doc["ativo"] != false {
		t.Fatalf("NDJSON: valor=%v ativo=%v", doc["valor"], doc["ativo"])
	}

	body := strings.TrimPrefix(csvBuf.String(), "\ufeff")
	recs, err := csv.NewReader(strings.NewReader(body)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(recs))
	}
	cell := map[string]string{}
	for i, h := range recs[0] {
		cell[h] = recs[1][i]
	}
	if cell["valor"] != "10.5" {
		t.Errorf("CSV valor: %q", cell["valor"])
	}
	if cell["ativo"] != "false" {
		t.Errorf("CSV active: %q — must match NDJSON, not become 0/1", cell["ativo"])
	}
	// the defect csv.ts documents: a quote must be doubled, not backslashed
	if cell["titulo"] != `Rua "do Meio", 3` {
		t.Errorf("CSV titulo: %q", cell["titulo"])
	}
	if !strings.HasPrefix(csvBuf.String(), "\ufeff") {
		t.Error("CSV needs BOM, or Excel misreads accents")
	}
	if !strings.Contains(body, "\r\n") {
		t.Error("CSV needs CRLF")
	}
}

// The manifest line is how a consumer knows the stream was not cut short.
func TestNDJSONSinkClosesWithTheManifest(t *testing.T) {
	e := setupExport(t)
	makeNotas(t, e, "Admin", "N", 3)

	var buf bytes.Buffer
	err := e.Run(context.Background(), "exp@x.com", func(c *Ctx) error {
		_, err := c.Export(ExportArgs{Doctype: "Nota"}, NewNDJSONSink(&buf))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 3 documents + manifest, got %d lines", len(lines))
	}
	var last map[string]json.RawMessage
	if err := json.Unmarshal([]byte(lines[3]), &last); err != nil {
		t.Fatal(err)
	}
	raw, ok := last["_manifest"]
	if !ok {
		t.Fatal("last line should be the manifest")
	}
	var sum ExportSummary
	if err := json.Unmarshal(raw, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Rows != 3 || sum.Doctype != "Nota" || sum.User != "exp@x.com" {
		t.Fatalf("manifest: %+v", sum)
	}
}

// Children go to their own CSV, keyed back to the parent.
func TestCSVSinkSplitsChildTables(t *testing.T) {
	e := setupExport(t)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Nota", Doc{"titulo": "Com itens"})
		d["itens"] = []any{map[string]any{"descricao": "a", "qtd": 1}, map[string]any{"descricao": "b", "qtd": 2}}
		_, err := c.Insert(d, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	out := map[string]*bytes.Buffer{}
	err = e.Run(context.Background(), "exp@x.com", func(c *Ctx) error {
		sink := &CSVSink{
			Open: func(table string) (io.Writer, error) {
				out[table] = &bytes.Buffer{}
				return out[table], nil
			},
			Lookup: func(n string) (*meta.DocType, error) { return c.St.DocType(n) },
		}
		_, err := c.Export(ExportArgs{Doctype: "Nota", Children: true}, sink)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if out["Nota"] == nil || out["Nota.itens"] == nil {
		t.Fatalf("expected one CSV per table, got %v", keysOf(out))
	}
	recs, err := csv.NewReader(strings.NewReader(strings.TrimPrefix(out["Nota.itens"].String(), "\ufeff"))).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("expected header + 2 children, got %d", len(recs))
	}
	if !contains(recs[0], "parent") || !contains(recs[0], "idx") {
		t.Errorf("child CSV needs parent/idx to relink to parent: %v", recs[0])
	}
}

func keysOf(m map[string]*bytes.Buffer) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}
