package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/testdb"
)

// importApp is the fixture for the restricted load path: metadata that must
// survive, a self link no order can satisfy, a submittable DocType with a
// series, a required Vault field that is never exported, a controller that
// would rewrite what it is given, and a date rule that would fire on history.
func importApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "imp", title: "Import Fixture", roles: ["Operador"] });`)
	w("doctypes/cliente/cliente.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cliente", idGeneration: { field: "code" },
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true },
    { fieldname: "nome", fieldtype: "Data", label: "Name" },
    { fieldname: "indicado_por", fieldtype: "Link", label: "Referred by", options: "Cliente" },
  ],
  permissions: [{ role: "Operador", read: true, write: true, create: true }] });`)
	w("doctypes/cliente/cliente.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController({
  validate(doc) { doc.nome = "REWRITTEN"; },
});`)
	w("doctypes/item_fatura/item_fatura.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Item Fatura", isChild: true, fields: [
  { fieldname: "descricao", fieldtype: "Data", label: "Description" },
  { fieldname: "preco", fieldtype: "Currency", label: "Price" } ] });`)
	w("doctypes/fatura/fatura.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Fatura", submittable: true, trackChanges: true,
  idGeneration: { series: "FAT-.####" },
  fields: [
    { fieldname: "cliente", fieldtype: "Link", label: "Customer", options: "Cliente" },
    { fieldname: "total", fieldtype: "Currency", label: "Total" },
    { fieldname: "vencimento", fieldtype: "Date", label: "Due date" },
    { fieldname: "itens", fieldtype: "Table", label: "Items", options: "Item Fatura" },
  ],
  permissions: [{ role: "Operador", read: true, write: true, create: true, submit: true }] });`)
	w("doctypes/integracao/integracao.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Integracao", fields: [
  { fieldname: "titulo", fieldtype: "Data", label: "Title", reqd: true },
  { fieldname: "token", fieldtype: "Vault", label: "Token", reqd: true },
  { fieldname: "senha", fieldtype: "Password", label: "Password", reqd: true },
 ], permissions: [{ role: "Operador", read: true, write: true, create: true }] });`)
	w("doctypes/categoria/categoria.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Categoria", isTree: true, idGeneration: { field: "titulo" },
  fields: [ { fieldname: "titulo", fieldtype: "Data", label: "Title", reqd: true } ],
  permissions: [{ role: "Operador", read: true, write: true, create: true }] });`)
	w("notifications/fatura_due.notification.ts", `import { defineNotification, _ } from "@ddcore/sdk";
export default defineNotification({
  name: "imp.fatura_due", doctype: "Fatura", date: { field: "vencimento", days: 0 },
  recipients: () => ["op@x.com"],
  desk: { title: (doc) => _("Invoice due: {0}", [doc.id]), message: () => _("Due today") },
});`)
	return dir
}

func setupImport(t *testing.T) *Engine {
	t.Helper()
	ctx := context.Background()
	// Its own database, so a load's tables never meet another test's: these
	// tests drop and recreate between runs.
	dsn := testdb.WithDatabase(testDSN, testdb.Database(testDSN)+"_imp")
	e := migratedEngine(t, Config{DSN: dsn, Apps: []js.App{{Name: "imp", Dir: importApp(t)}}, Test: true, DataDir: t.TempDir()})
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("User", Doc{"email": "op@x.com", "full_name": "Op"})
		d["roles"] = []any{map[string]any{"role": "Operador"}}
		_, err := c.Insert(d, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func importAs(t *testing.T, e *Engine, user string, docs ...Doc) error {
	t.Helper()
	return e.Run(context.Background(), user, func(c *Ctx) error {
		for _, doc := range docs {
			if _, err := c.ImportDoc(doc, ImportOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
}

func scalar(t *testing.T, e *Engine, sql string, args ...any) any {
	t.Helper()
	var v any
	if err := e.DB.Pool.QueryRow(context.Background(), sql, args...).Scan(&v); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
	// Postgres hands back int32 for an integer column and int64 for a bigint;
	// the assertions care about the number, not its width.
	switch n := v.(type) {
	case int32:
		return int64(n)
	case int16:
		return int64(n)
	}
	return v
}

// The whole point of the restricted path: what the export carries is what the
// database ends up holding.
func TestImportDocKeepsIdentityAndMetadata(t *testing.T) {
	e := setupImport(t)
	created := time.Date(2019, 3, 4, 10, 30, 0, 0, time.UTC)
	doc := Doc{"doctype": "Cliente", "id": "C-LEGADO-7", "code": "C-LEGADO-7", "nome": "Acme",
		"owner": "op@x.com", "creation": created.Format(time.RFC3339), "modified": created.Format(time.RFC3339),
		"modified_by": "op@x.com", "docstatus": 0}
	if err := importAs(t, e, "Admin", doc); err != nil {
		t.Fatal(err)
	}
	row := func(col string) any {
		return scalar(t, e, "SELECT "+col+" FROM tab_cliente WHERE id = $1", "C-LEGADO-7")
	}
	if row("owner") != "op@x.com" {
		t.Fatalf("owner = %v", row("owner"))
	}
	if got := row("creation").(time.Time); !got.Equal(created) {
		t.Fatalf("creation = %v, want %v", got, created)
	}
	if row("nome") != "Acme" {
		t.Fatalf("the controller's validate hook ran: nome = %v", row("nome"))
	}
}

// A cancelled document cannot be reached through Insert at all; history has
// plenty of them.
func TestImportDocLoadsEveryDocstatus(t *testing.T) {
	e := setupImport(t)
	docs := []Doc{
		{"doctype": "Fatura", "id": "FAT-0001", "total": 10, "docstatus": 0},
		{"doctype": "Fatura", "id": "FAT-0002", "total": 20, "docstatus": 1},
		{"doctype": "Fatura", "id": "FAT-0003", "total": 30, "docstatus": 2},
	}
	if err := importAs(t, e, "Admin", docs...); err != nil {
		t.Fatal(err)
	}
	for id, want := range map[string]int64{"FAT-0001": 0, "FAT-0002": 1, "FAT-0003": 2} {
		if got := scalar(t, e, "SELECT docstatus FROM tab_fatura WHERE id = $1", id); got != want {
			t.Fatalf("%s docstatus = %v, want %d", id, got, want)
		}
	}
}

// Nothing an import writes may leave the site or be replayed: the documents
// are history, and their effects already happened.
func TestImportDocFiresNoEffects(t *testing.T) {
	e := setupImport(t)
	doc := Doc{"doctype": "Fatura", "id": "FAT-0009", "total": 5, "docstatus": 1,
		"itens": []any{map[string]any{"doctype": "Item Fatura", "id": "row1", "descricao": "x", "preco": 5, "idx": 1}}}
	if err := importAs(t, e, "Admin", doc); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		"SELECT count(*) FROM tab_version",
		"SELECT count(*) FROM ddcore_job",
		"SELECT count(*) FROM ddcore_notification",
	} {
		if got := scalar(t, e, q); got != int64(0) {
			t.Fatalf("%s = %v", q, got)
		}
	}
	if got := scalar(t, e, "SELECT count(*) FROM tab_item_fatura WHERE parent = $1", "FAT-0009"); got != int64(1) {
		t.Fatalf("child rows = %v", got)
	}
}

// A child row's own id and idx are part of the identity being preserved.
func TestImportDocKeepsChildIdentity(t *testing.T) {
	e := setupImport(t)
	doc := Doc{"doctype": "Fatura", "id": "FAT-0010", "total": 5,
		"itens": []any{
			map[string]any{"doctype": "Item Fatura", "id": "legado-b", "descricao": "b", "idx": 2},
			map[string]any{"doctype": "Item Fatura", "id": "legado-a", "descricao": "a", "idx": 1},
		}}
	if err := importAs(t, e, "Admin", doc); err != nil {
		t.Fatal(err)
	}
	// The rows arrive in whatever order the line held them; their own idx is
	// what orders them, and it is renumbered densely from there.
	if got := scalar(t, e, "SELECT idx FROM tab_item_fatura WHERE id = $1", "legado-b"); got != int64(2) {
		t.Fatalf("legado-b idx = %v", got)
	}
	if got := scalar(t, e, "SELECT idx FROM tab_item_fatura WHERE id = $1", "legado-a"); got != int64(1) {
		t.Fatalf("legado-a idx = %v", got)
	}
}

// A self link cannot be satisfied while the set is still loading.
func TestImportDocDefersALinkWhenAsked(t *testing.T) {
	e := setupImport(t)
	doc := Doc{"doctype": "Cliente", "id": "C-1", "code": "C-1", "indicado_por": "C-2"}
	if err := importAs(t, e, "Admin", doc); err == nil {
		t.Fatal("a link to a document that is not there yet must fail when checked inline")
	}
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		_, err := c.ImportDoc(doc, ImportOpts{Deferred: func(dt, field string) bool { return field == "indicado_por" }})
		return err
	})
	if err != nil {
		t.Fatalf("deferred: %v", err)
	}
}

// Neither a Password nor a Vault field is ever exported, so a required one can
// only fail — the load reports it instead.
func TestImportDocReportsSecretsItCannotCarry(t *testing.T) {
	e := setupImport(t)
	var notes []string
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		res, err := c.ImportDoc(Doc{"doctype": "Integracao", "id": "I-1", "titulo": "ERP"}, ImportOpts{})
		if err != nil {
			return err
		}
		notes = res.Notes
		return nil
	})
	if err != nil {
		t.Fatalf("a missing secret must not fail the record: %v", err)
	}
	if len(notes) != 2 {
		t.Fatalf("notes = %v; both the Vault and the Password field are not migrated", notes)
	}
}

// The first document created after a load must not take an id the load wrote.
func TestImportDocAdvancesTheSeriesCounter(t *testing.T) {
	e := setupImport(t)
	if err := importAs(t, e, "Admin", Doc{"doctype": "Fatura", "id": "FAT-0042", "total": 1}); err != nil {
		t.Fatal(err)
	}
	var next string
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, _ := c.NewDoc("Fatura", Doc{"total": 2})
		saved, err := c.Insert(d, SaveOpts{IgnorePermissions: true})
		if err != nil {
			return err
		}
		next = saved.ID()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if next != "FAT-0043" {
		t.Fatalf("next id = %q; the counter did not follow the load", next)
	}
}

// A date rule sweeping historical documents would notify people about invoices
// that came due years ago.
func TestImportDocDoesNotNotifyHistory(t *testing.T) {
	e := setupImport(t)
	past := time.Now().AddDate(-2, 0, 0).Format("2006-01-02")
	if err := importAs(t, e, "Admin", Doc{"doctype": "Fatura", "id": "FAT-0100", "total": 1, "vencimento": past}); err != nil {
		t.Fatal(err)
	}
	if err := e.SweepNotifications(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	if got := scalar(t, e, "SELECT count(*) FROM ddcore_notification"); got != int64(0) {
		t.Fatalf("notifications = %v; a past due date must arrive already marked", got)
	}
}

func TestImportDocRefusesWhatItCannotLoad(t *testing.T) {
	e := setupImport(t)
	cases := []struct {
		name string
		doc  Doc
	}{
		{"a child table", Doc{"doctype": "Item Fatura", "id": "x"}},
		{"the audit ledger", Doc{"doctype": "Audit Event", "id": "x"}},
		{"a document without an id", Doc{"doctype": "Cliente", "code": "C-9"}},
	}
	for _, tc := range cases {
		if err := importAs(t, e, "Admin", tc.doc); err == nil {
			t.Fatalf("%s should be refused", tc.name)
		}
	}
}

// The path forges owner and creation, so it belongs to whoever administers the
// site, never to an ordinary role.
func TestImportDocIsAdminOnly(t *testing.T) {
	e := setupImport(t)
	err := importAs(t, e, "op@x.com", Doc{"doctype": "Cliente", "id": "C-5", "code": "C-5"})
	if err == nil {
		t.Fatal("an ordinary user must not import")
	}
}

// Everything else a load writes is history, but a role is a live grant on this
// site from the moment it lands — and skipping the hooks skipped the audit the
// User controller writes.
func TestImportDocAuditsTheAccessItGrants(t *testing.T) {
	e := setupImport(t)
	doc := Doc{"doctype": "User", "id": "importado@x.com", "email": "importado@x.com",
		"full_name": "Importado", "enabled": true,
		"roles": []any{map[string]any{"doctype": "Has Role", "id": "hr1", "role": "Operador"}}}
	if err := importAs(t, e, "Admin", doc); err != nil {
		t.Fatal(err)
	}
	got := scalar(t, e, `SELECT count(*) FROM tab_audit_event WHERE action = 'role.assign' AND target_id = $1`, "importado@x.com")
	if got != int64(1) {
		t.Fatalf("role.assign events = %v", got)
	}
	if d := scalar(t, e, `SELECT detail::text FROM tab_audit_event WHERE action = 'role.assign' AND target_id = $1`, "importado@x.com"); !strings.Contains(d.(string), "import") {
		t.Fatalf("detail = %v; the event should say where the grant came from", d)
	}
}

// A hierarchy arrives in whatever order the export walked it, so a node often
// precedes its parent — but a parent that is not a group is broken data, and
// the load has to say so.
func TestImportDocKeepsTreeInvariants(t *testing.T) {
	e := setupImport(t)
	deferParent := ImportOpts{Deferred: func(doctype, field string) bool { return field == "parent_categoria" }}
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		// The child comes first: its parent is not loaded yet.
		if _, err := c.ImportDoc(Doc{"doctype": "Categoria", "id": "C-filha", "titulo": "Filha",
			"parent_categoria": "C-mae"}, deferParent); err != nil {
			return err
		}
		if _, err := c.ImportDoc(Doc{"doctype": "Categoria", "id": "C-mae", "titulo": "Mãe", "is_group": true}, deferParent); err != nil {
			return err
		}
		// A leaf cannot take children, whichever order they arrive in.
		_, err := c.ImportDoc(Doc{"doctype": "Categoria", "id": "C-neta", "titulo": "Neta",
			"parent_categoria": "C-filha"}, deferParent)
		if err == nil {
			return fmt.Errorf("a parent that is not a group must be refused")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
