package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
)

// bulkFiles adds the DocTypes the bulk writes are tried on: Espelho is a
// mirror row (a unique key, a Link, a fetched field, a Select, a default),
// Lote numbers its ids from a series.
var bulkFiles = map[string]string{
	"doctypes/espelho/espelho.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Espelho", idGeneration: { field: "chave" },
  fields: [
    { fieldname: "chave", fieldtype: "Data", label: "Chave", reqd: true, unique: true },
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "preco", fieldtype: "Currency", label: "Preço" },
    { fieldname: "qtd", fieldtype: "Int", label: "Qtd", default: 1 },
    { fieldname: "tipo", fieldtype: "Select", label: "Tipo", options: ["A", "B"] },
    { fieldname: "pessoa", fieldtype: "Link", label: "Pessoa", options: "Pessoa" },
    { fieldname: "pessoa_cpf", fieldtype: "Data", label: "CPF", fetchFrom: "pessoa.cpf", readOnly: true },
    { fieldname: "removido_em", fieldtype: "Date", label: "Removido em" },
    { fieldname: "selo", fieldtype: "Data", label: "Selo", setOnlyOnce: true },
    { fieldname: "anexo", fieldtype: "Attach", label: "Anexo" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`,
	"doctypes/lote/lote.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Lote", idGeneration: { series: "LT-.####" },
  fields: [{ fieldname: "descricao", fieldtype: "Data", label: "Descrição" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`,
}

func setupBulk(t *testing.T) *Engine {
	t.Helper()
	return setupWith(t, bulkFiles)
}

// evalBulk runs code as user in a transaction and decodes what it returns.
// A thrown error comes back as the error.
func evalBulk(e *Engine, user, code string, out any) error {
	return e.Run(context.Background(), user, func(c *Ctx) error {
		rt, err := c.RT()
		if err != nil {
			return err
		}
		raw, err := rt.Eval(code)
		if err != nil {
			return err
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(raw, out)
	})
}

func mustEvalBulk(t *testing.T, e *Engine, code string, out any) {
	t.Helper()
	if err := evalBulk(e, "Admin", code, out); err != nil {
		t.Fatal(err)
	}
}

// wantBulkErr asserts err is a ddcore error of type typ whose message holds
// msg, and (row >= 0) that it names row.
func wantBulkErr(t *testing.T, err error, typ, msg string, row int) {
	t.Helper()
	if err == nil {
		t.Fatalf("want %s %q, got success", typ, msg)
	}
	e := cerr.From(err)
	if e.Type != typ || !strings.Contains(e.Message, msg) {
		t.Fatalf("want %s %q, got %s %q", typ, msg, e.Type, e.Message)
	}
	if row >= 0 {
		m, _ := e.Extra.(map[string]any)
		if got, ok := m["row"]; !ok || fmt.Sprint(got) != fmt.Sprint(row) {
			t.Fatalf("want row %d, got extra %#v", row, e.Extra)
		}
	}
}

func espelhos(t *testing.T, e *Engine) map[string]Doc {
	t.Helper()
	out := map[string]Doc{}
	runAs(t, e, "Admin", func(c *Ctx) error {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT * FROM tab_espelho`)
		for _, r := range rows {
			out[db.Str(r["id"])] = Doc(r)
		}
		return err
	})
	return out
}

func TestBulkInsertMany(t *testing.T) {
	e := setupBulk(t)
	var n int
	mustEvalBulk(t, e, `
ddcore.newDoc("Pessoa", { nome: "Ana", cpf: "111" }).insert();
ddcore.db.insertMany("Espelho", [
  { chave: "  k1 ", nome: "Um", preco: "10.5", tipo: "A", pessoa: "Ana" },
  { chave: "k2", nome: "Dois", qtd: 7 },
])`, &n)
	if n != 2 {
		t.Fatalf("inserted %d, want 2", n)
	}
	rows := espelhos(t, e)
	k1, k2 := rows["k1"], rows["k2"]
	if k1 == nil || k2 == nil {
		t.Fatalf("rows: %v", rows)
	}
	if toFloat(k1["preco"]) != 10.5 || toFloat(k1["qtd"]) != 1 || k1.Str("pessoa_cpf") != "111" || k1.Str("owner") != "Admin" || k1["modified"] == nil {
		t.Fatalf("k1 = %v", k1)
	}
	if toFloat(k2["qtd"]) != 7 || k2["pessoa_cpf"] != nil {
		t.Fatalf("k2 = %v", k2)
	}
	// the documents read back as any other
	var doc map[string]any
	mustEvalBulk(t, e, `ddcore.getDoc("Espelho", "k1")`, &doc)
	if doc["nome"] != "Um" {
		t.Fatalf("getDoc = %v", doc)
	}
}

func TestBulkInsertManyChecksEveryRow(t *testing.T) {
	e := setupBulk(t)
	cases := []struct {
		name, rows, typ, msg string
		row                  int
	}{
		{"reqd", `[{ chave: "a", nome: "A" }, { chave: "b" }]`, "MandatoryError", "Nome", 1},
		{"select", `[{ chave: "a", nome: "A", tipo: "Z" }]`, "ValidationError", "not one of the options", 0},
		{"link", `[{ chave: "a", nome: "A" }, { chave: "b", nome: "B", pessoa: "Nobody" }]`, "LinkExistsError", "Nobody", 1},
		{"duplicate in call", `[{ chave: "a", nome: "A" }, { chave: "a", nome: "B" }]`, "DuplicateEntryError", "a", 1},
		{"unknown field", `[{ chave: "a", nome: "A", nada: 1 }]`, "ValidationError", "nada", -1},
		{"std column", `[{ chave: "a", nome: "A", owner: "x" }]`, "ValidationError", "owner", -1},
		{"attach", `[{ chave: "a", nome: "A", anexo: "/f" }]`, "ValidationError", "Attach", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := evalBulk(e, "Admin", `ddcore.db.insertMany("Espelho", `+tc.rows+`)`, nil)
			wantBulkErr(t, err, tc.typ, tc.msg, tc.row)
		})
	}
	if rows := espelhos(t, e); len(rows) != 0 {
		t.Fatalf("a refused call wrote %v", rows)
	}
	// the row reaches the app's code as e.extra.row
	var row any
	mustEvalBulk(t, e, `let row = null;
try { ddcore.db.insertMany("Espelho", [{ chave: "a", nome: "A" }, { chave: "b" }]); } catch (e) { row = e.extra && e.extra.row; }
row`, &row)
	if row != float64(1) {
		t.Fatalf("e.extra.row = %v, want 1", row)
	}
	// ignoreLinks lets a dangling Link through, as on insert
	if err := evalBulk(e, "Admin", `ddcore.db.insertMany("Espelho", [{ chave: "b", nome: "B", pessoa: "Nobody" }], { ignoreLinks: true })`, nil); err != nil {
		t.Fatal(err)
	}
	// a stored row's key is the index's to refuse, catchable in a savepoint
	var got []any
	mustEvalBulk(t, e, `
const out = [];
try { ddcore.db.savepoint(() => ddcore.db.insertMany("Espelho", [{ chave: "c", nome: "C" }, { chave: "b", nome: "B2" }])); }
catch (e) { out.push(e.name, e.message); }
out.push(ddcore.db.count("Espelho", {}));
out`, &got)
	if len(got) != 3 || got[0] != "DuplicateEntryError" || got[2] != float64(1) {
		t.Fatalf("got %v", got)
	}
}

func TestBulkRefusals(t *testing.T) {
	e := setupBulk(t)
	cases := []struct{ name, code, typ, msg string }{
		{"child", `ddcore.db.insertMany("Item Pedido", [{ descricao: "x" }])`, "ValidationError", "cannot be written in bulk"},
		{"core", `ddcore.db.insertMany("User", [{ email: "x@y.z" }])`, "ValidationError", "cannot be written in bulk"},
		{"trackChanges update", `ddcore.db.updateMany("Pessoa", [{ id: "x", values: { cpf: "1" } }])`, "ValidationError", "tracks changes"},
		{"trackChanges upsert", `ddcore.db.upsertMany("Pessoa", [{ nome: "x" }], { key: "id" })`, "ValidationError", "tracks changes"},
		{"no key", `ddcore.db.upsertMany("Espelho", [{ chave: "x", nome: "x" }], {})`, "ValidationError", "upsert key"},
		{"key not unique", `ddcore.db.upsertMany("Espelho", [{ chave: "x", nome: "x" }], { key: "nome" })`, "ValidationError", "upsert key"},
		{"counter upsert", `ddcore.db.upsertMany("Lote", [{ descricao: "x" }], { key: "id" })`, "ValidationError", "counter"},
		{"setOnlyOnce update", `ddcore.db.updateMany("Espelho", [{ id: "x", values: { selo: "s" } }])`, "ValidationError", "only be set once"},
		{"id in update", `ddcore.db.updateMany("Espelho", [{ id: "x", values: { id: "y" } }])`, "ValidationError", "id"},
		{"too many", `ddcore.db.insertMany("Espelho", Array.from({ length: 10001 }, (_, i) => ({ chave: "k" + i, nome: "n" })))`, "ValidationError", "10000"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			wantBulkErr(t, evalBulk(e, "Admin", tc.code, nil), tc.typ, tc.msg, -1)
		})
	}
}

func TestBulkPermissions(t *testing.T) {
	e := setupBulk(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		// leitor has no role that may create an Espelho
		if err := insertDoc(c, "User", Doc{"email": "leitor@x.test", "full_name": "Leitor"}); err != nil {
			return err
		}
		if err := insertDoc(c, "User", Doc{"email": "gestor@x.test", "full_name": "Gestor", "roles": []any{map[string]any{"role": "Gestor"}}}); err != nil {
			return err
		}
		return insertDoc(c, "User Permission", Doc{"user": "gestor@x.test", "allow": "Pessoa", "for_value": "Ana"})
	})
	ins := `ddcore.db.insertMany("Espelho", [{ chave: "a", nome: "A" }]%s)`
	wantBulkErr(t, evalBulk(e, "leitor@x.test", fmt.Sprintf(ins, ""), nil), "PermissionError", "create", -1)
	if err := evalBulk(e, "leitor@x.test", fmt.Sprintf(ins, ", { ignorePermissions: true }"), nil); err != nil {
		t.Fatal(err)
	}
	// a scoped user writes one document at a time, whatever the options say
	wantBulkErr(t, evalBulk(e, "gestor@x.test", strings.Replace(fmt.Sprintf(ins, ", { ignorePermissions: true }"), `"a"`, `"b"`, 1), nil), "PermissionError", "access scopes", -1)
}

func TestBulkSeriesReservesABlock(t *testing.T) {
	e := setupBulk(t)
	var ids []string
	mustEvalBulk(t, e, `
ddcore.newDoc("Lote", { descricao: "antes" }).insert();
ddcore.db.insertMany("Lote", [{ descricao: "a" }, { descricao: "b" }, { descricao: "c" }]);
ddcore.newDoc("Lote", { descricao: "depois" }).insert();
ddcore.db.getAll("Lote", { fields: ["id"], orderBy: "id asc" }).map(r => r.id)`, &ids)
	want := []string{"LT-0001", "LT-0002", "LT-0003", "LT-0004", "LT-0005"}
	if strings.Join(ids, ",") != strings.Join(want, ",") {
		t.Fatalf("ids %v, want %v", ids, want)
	}
}

func TestBulkUpdateMany(t *testing.T) {
	e := setupBulk(t)
	mustEvalBulk(t, e, `ddcore.newDoc("Pessoa", { nome: "Ana", cpf: "111" }).insert();
ddcore.db.insertMany("Espelho", [{ chave: "a", nome: "A", preco: 1 }, { chave: "b", nome: "B", preco: 2 }, { chave: "c", nome: "C", preco: 3 }])`, nil)
	before := espelhos(t, e)
	time.Sleep(10 * time.Millisecond)
	var n int
	mustEvalBulk(t, e, `ddcore.db.updateMany("Espelho", [
  { id: "a", values: { preco: 1 } },
  { id: "b", values: { preco: 20 } },
  { id: "c", values: { nome: "Cê", pessoa: "Ana" } },
])`, &n)
	if n != 2 {
		t.Fatalf("updated %d, want 2", n)
	}
	after := espelhos(t, e)
	if !sameTime(after["a"]["modified"], before["a"]["modified"], time.UTC) {
		t.Fatalf("an unchanged row was rewritten: %v → %v", before["a"]["modified"], after["a"]["modified"])
	}
	if toFloat(after["b"]["preco"]) != 20 || sameTime(after["b"]["modified"], before["b"]["modified"], time.UTC) {
		t.Fatalf("b = %v", after["b"])
	}
	if after["c"].Str("nome") != "Cê" || after["c"].Str("pessoa_cpf") != "111" {
		t.Fatalf("c = %v (the link's fetched field follows it)", after["c"])
	}
	wantBulkErr(t, evalBulk(e, "Admin", `ddcore.db.updateMany("Espelho", [{ id: "a", values: { nome: "x" } }, { id: "zz", values: { nome: "y" } }])`, nil), "DoesNotExistError", "zz", 1)
	wantBulkErr(t, evalBulk(e, "Admin", `ddcore.db.updateMany("Espelho", [{ id: "a", values: { nome: "" } }])`, nil), "MandatoryError", "Nome", 0)
	wantBulkErr(t, evalBulk(e, "Admin", `ddcore.db.updateMany("Espelho", [{ id: "a", values: { chave: "b" } }])`, nil), "DuplicateEntryError", "b", -1)
}

func TestBulkUpsertMany(t *testing.T) {
	e := setupBulk(t)
	mustEvalBulk(t, e, `ddcore.db.insertMany("Espelho", [
  { chave: "a", nome: "A", preco: 1 },
  { chave: "b", nome: "B", preco: 2, removido_em: "2026-01-01" },
])`, nil)
	before := espelhos(t, e)
	time.Sleep(10 * time.Millisecond)
	var res UpsertResult
	mustEvalBulk(t, e, `ddcore.db.upsertMany("Espelho", [
  { chave: "a", nome: "A", preco: 1 },
  { chave: "b", nome: "B", preco: 2, removido_em: null },
  { chave: "c", nome: "C", preco: 3 },
], { key: "chave" })`, &res)
	if res != (UpsertResult{Inserted: 1, Updated: 1, Unchanged: 1}) {
		t.Fatalf("res = %+v", res)
	}
	after := espelhos(t, e)
	if !sameTime(after["a"]["modified"], before["a"]["modified"], time.UTC) {
		t.Fatal("an unchanged row was rewritten")
	}
	if after["b"]["removido_em"] != nil || sameTime(after["b"]["modified"], before["b"]["modified"], time.UTC) {
		t.Fatalf("b = %v", after["b"])
	}
	if after["c"] == nil || toFloat(after["c"]["qtd"]) != 1 {
		t.Fatalf("c = %v", after["c"])
	}
	// a field the row leaves out keeps its stored value: only what it brings is compared
	mustEvalBulk(t, e, `ddcore.db.setValue("Espelho", "a", "qtd", 9);
ddcore.db.upsertMany("Espelho", [{ chave: "a", nome: "A2" }], { key: "chave" })`, &res)
	if res != (UpsertResult{Updated: 1}) {
		t.Fatalf("res = %+v", res)
	}
	if a := espelhos(t, e)["a"]; a.Str("nome") != "A2" || toFloat(a["qtd"]) != 9 || toFloat(a["preco"]) != 1 {
		t.Fatalf("a = %v", a)
	}
	// by id
	mustEvalBulk(t, e, `ddcore.db.upsertMany("Espelho", [{ chave: "a", nome: "A3" }, { chave: "d", nome: "D" }], { key: "id" })`, &res)
	if res != (UpsertResult{Inserted: 1, Updated: 1}) {
		t.Fatalf("res = %+v", res)
	}
	wantBulkErr(t, evalBulk(e, "Admin", `ddcore.db.upsertMany("Espelho", [{ chave: "x", nome: "1" }, { chave: "x", nome: "2" }], { key: "chave" })`, nil), "DuplicateEntryError", "x", 1)
	wantBulkErr(t, evalBulk(e, "Admin", `ddcore.db.upsertMany("Espelho", [{ chave: "x", nome: "1", selo: "s" }], { key: "chave" })`, nil), "ValidationError", "only be set once", -1)
}

// Each tenant upserts its own rows: the conflict target leads with the
// tenant, so the same key in two tenants is two rows.
func TestBulkUpsertManyInTenants(t *testing.T) {
	files := map[string]string{}
	for k, v := range tenancyFiles {
		files[k] = v
	}
	for k, v := range bulkFiles {
		files[k] = v
	}
	e := migratedEngine(t, Config{Apps: []js.App{{Name: "demo", Dir: testApp(t, files)}}, Test: true, Tenancy: true})
	seedTenants(t, e)
	upsert := func(tenant, nome string) UpsertResult {
		var res UpsertResult
		inTenant(t, e, tenant, func(c *Ctx) error {
			var err error
			res, err = c.UpsertMany("Espelho", []Doc{{"chave": "k", "nome": nome}, {"chave": "j", "nome": "J"}}, BulkOpts{Key: "chave"})
			return err
		})
		return res
	}
	if r := upsert(tenantA, "A"); r != (UpsertResult{Inserted: 2}) {
		t.Fatalf("alfa: %+v", r)
	}
	if r := upsert(tenantB, "B"); r != (UpsertResult{Inserted: 2}) {
		t.Fatalf("beta: %+v", r)
	}
	if r := upsert(tenantA, "A2"); r != (UpsertResult{Updated: 1, Unchanged: 1}) {
		t.Fatalf("alfa again: %+v", r)
	}
	var by []map[string]any
	runAs(t, e, "Admin", func(c *Ctx) error {
		return c.elevated(func() error {
			rows, err := db.Select(c.Ctx, c.Q(), `SELECT tenant, id, nome FROM tab_espelho ORDER BY 1, 2`)
			by = rows
			return err
		})
	})
	got := []string{}
	for _, r := range by {
		got = append(got, db.Str(r["tenant"])+"/"+db.Str(r["id"])+"="+db.Str(r["nome"]))
	}
	want := "alfa/j=J,alfa/k=A2,beta/j=J,beta/k=B"
	if strings.Join(got, ",") != want {
		t.Fatalf("rows %v, want %s", got, want)
	}
}

// A tenant's server code adds to a DocType the platform keeps in the platform
// space, as insert does; updateMany stays refused there, as setValue is.
func TestBulkPlatformDocTypeFromATenant(t *testing.T) {
	e := setupTenantAccess(t)
	inTenant(t, e, tenantA, func(c *Ctx) error {
		n, err := c.InsertMany("Protocolo", []Doc{{"assunto": "a"}, {"assunto": "b"}}, BulkOpts{IgnorePermissions: true})
		if err != nil || n != 2 {
			t.Fatalf("insertMany = %d, %v", n, err)
		}
		if c.Tenant != tenantA {
			t.Fatalf("the write left the ctx in %q", c.Tenant)
		}
		_, err = c.UpdateMany("Protocolo", []BulkChange{{ID: "PRT-0001", Values: Doc{"assunto": "x"}}}, BulkOpts{IgnorePermissions: true})
		wantStatus(t, err, 403)
		return nil
	})
	runAs(t, e, "Admin", func(c *Ctx) error {
		rows, err := db.Select(c.Ctx, c.Q(), `SELECT id, assunto FROM tab_protocolo ORDER BY id`)
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0]["id"] != "PRT-0001" || rows[1]["assunto"] != "b" {
			t.Fatalf("platform rows = %v", rows)
		}
		return nil
	})
}
