package engine

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// testDSN comes from DDCORE_TEST_DSN; the database named in it is dropped and
// recreated by setup, so point it at a throwaway database.
var testDSN = envOr("DDCORE_TEST_DSN", "postgres://ddcore:ddcore@localhost:5455/ddcore_test?sslmode=disable")

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

// adminDSNFor swaps the database of a DSN for "postgres" (to create/drop the test db)
// and returns the test database name.
func adminDSNFor(dsn string) (adminDSN, dbName string) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", ""
	}
	dbName = strings.TrimPrefix(u.Path, "/")
	u.Path = "/postgres"
	return u.String(), dbName
}

func testApp(t *testing.T, extra ...map[string]string) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}

	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"], docEvents: { "*": { validate(doc) { doc.flags.seen = true } } } });`)
	w("doctypes/pessoa/pessoa.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" }, allowRename: true, trackChanges: true, searchFields: ["cpf"],
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true },
    { fieldname: "cpf", fieldtype: "Data", label: "CPF", unique: true },
    { fieldname: "email", fieldtype: "Email", label: "E-mail" },
    { fieldname: "tipo", fieldtype: "Select", label: "Tipo", options: ["PF", "PJ"], default: "PF" },
    { fieldname: "limite", fieldtype: "Currency", label: "Limite" },
    { fieldname: "codigo", fieldtype: "Data", label: "Código", readOnlyDependsOn: "doc.tipo == 'PJ'" },
    { fieldname: "segredo", fieldtype: "Password", label: "Segredo" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }, { role: "All", read: true, ifOwner: true }] });`)
	w("doctypes/item/item.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Item Pedido", isChild: true, fields: [
  { fieldname: "descricao", fieldtype: "Data", label: "Descrição", reqd: true },
  { fieldname: "qtd", fieldtype: "Int", label: "Qtd", default: 1 },
  { fieldname: "valor", fieldtype: "Currency", label: "Valor" } ] });`)
	w("doctypes/pedido/pedido.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pedido", naming: { series: "PED-.YYYY.-.####" }, submittable: true, trackChanges: true,
  fields: [
    { fieldname: "cliente", fieldtype: "Link", label: "Cliente", options: "Pessoa", reqd: true },
    { fieldname: "cliente_tipo", fieldtype: "Data", label: "Tipo do cliente", fetchFrom: "cliente.tipo", readOnly: true },
    { fieldname: "obs", fieldtype: "Small Text", label: "Obs", allowOnSubmit: true },
    { fieldname: "desconto", fieldtype: "Currency", label: "Desconto", mandatoryDependsOn: "doc.total > 1000" },
    { fieldname: "total", fieldtype: "Currency", label: "Total", readOnly: true },
    { fieldname: "itens", fieldtype: "Table", label: "Itens", options: "Item Pedido" },
    { fieldname: "amended_from", fieldtype: "Link", label: "Emenda de", options: "Pedido", readOnly: true },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true }] });`)
	w("doctypes/pedido/pedido.controller.ts", `import { defineController, _ } from "@ddcore/sdk";
export default defineController("Pedido", {
  validate(doc) {
    doc.total = (doc.itens || []).reduce((s, i) => s + ddcore.utils.flt(i.qtd) * ddcore.utils.flt(i.valor), 0);
    if (doc.total < 0) ddcore.throw(_("Total negativo"), { title: "Pedido" });
    if (doc.obs === "bagunca") doc.desconto = ddcore.utils.flt(doc.desconto) + 1;
  },
  onSubmit(doc) { ddcore.db.setValue("Pessoa", doc.cliente, "limite", doc.total); },
  methods: {
    resumo(doc, args) { return { itens: doc.itens.length, total: doc.total, x: args.x }; },
    tocar(doc) { doc.dbSet("obs", "tocado"); doc.save(); return { obs: doc.obs }; },
  },
});`)
	w("services/loop.ts", `export function travar() { let n = 0; while (true) { n++ } }
export function ok(args: any) { return { ok: true, x: args.x } }`)
	w("doctypes/pedido/pedido.test.ts", `import "@ddcore/sdk/test";
describe("Pedido", () => {
  it("soma os itens", () => {
    ddcore.newDoc("Pessoa", { nome: "Teste", tipo: "PF" }).insert();
    const p = ddcore.newDoc("Pedido", { cliente: "Teste" });
    p.append("itens", { descricao: "a", qtd: 2, valor: 10 });
    p.insert();
    expect(p.total).toBe(20);
    expect(p.name).toMatch(/^PED-/);
  });
  it("falha sem cliente", () => { expect(() => ddcore.newDoc("Pedido").insert()).toThrow("required fields"); });
});`)
	// Last, so a test can override a file of the base app as well as add one —
	// a patches/ directory, most of the time.
	for _, files := range extra {
		for rel, src := range files {
			w(rel, src)
		}
	}
	return dir
}

func setup(t *testing.T) *Engine { return setupWith(t, nil) }

// setupWith is setup with extra files planted in the test app — how the patch
// tests get a patches/ directory without every other test paying for one.
func setupWith(t *testing.T, extra map[string]string) *Engine {
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	if dbName == "" {
		t.Fatalf("DDCORE_TEST_DSN inválida: %s", testDSN)
	}
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
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "demo", Dir: testApp(t, extra)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	plan, _ := e.Plan(ctx, false)
	if len(plan) != 0 {
		t.Fatalf("migrate não é idempotente: %v", plan)
	}
	t.Cleanup(func() { e.DB.Close() })
	return e
}

func TestLifecycle(t *testing.T) {
	e := setup(t)
	ctx := context.Background()
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		// user + roles
		u, _ := c.NewDoc("User", Doc{"email": "ana@x.com", "full_name": "Ana", "new_password": "segredo123"})
		u["roles"] = []any{map[string]any{"role": "Gestor"}}
		u, err := c.Insert(u, SaveOpts{})
		if err != nil {
			return fmt.Errorf("linha 109: %w", err)
		}
		if !CheckPassword(u.Str("password_hash"), "segredo123") || u.Str("new_password") != "" {
			t.Fatalf("senha não foi hasheada: %v", u)
		}
		invalidUser, _ := c.NewDoc("User", Doc{"email": "invalid", "full_name": "Inválido"})
		if _, err := c.Insert(invalidUser, SaveOpts{}); err == nil || cerr.From(err).Title != "Invalid email" {
			t.Fatalf("esperava validação central de email em User, veio %v", err)
		}
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Ana", "cpf": "123", "tipo": "PJ", "email": "  ana+teste@example.com  "})
		p, err = c.Insert(p, SaveOpts{})
		if err != nil {
			return fmt.Errorf("linha 116: %w", err)
		}
		if p.Str("email") != "ana+teste@example.com" {
			t.Fatalf("email não foi normalizado: %q", p.Str("email"))
		}
		invalid, _ := c.NewDoc("Pessoa", Doc{"nome": "Email inválido", "email": "invalid"})
		if _, err := c.Insert(invalid, SaveOpts{}); err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("esperava ValidationError de email no insert, veio %v", err)
		}
		p["email"] = "invalid"
		if _, err := c.Save(p, SaveOpts{}); err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("esperava ValidationError de email no update, veio %v", err)
		}
		p["email"] = "ana+teste@example.com"
		p2, _ := c.NewDoc("Pessoa", Doc{"nome": "Bia", "cpf": "123"})
		if _, err := c.Insert(p2, SaveOpts{}); err == nil || cerr.From(err).Type != "DuplicateEntryError" {
			t.Fatalf("esperava duplicidade de cpf, veio %v", err)
		}
		p3, _ := c.NewDoc("Pessoa", Doc{"nome": "Cid", "tipo": "XX"})
		if _, err := c.Insert(p3, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "is not one of the options") {
			t.Fatalf("esperava erro de select, veio %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// as Ana (Gestor)
	var name string
	err = e.Run(ctx, "ana@x.com", func(c *Ctx) error {
		ped, _ := c.NewDoc("Pedido", Doc{"cliente": "Ana"})
		ped["itens"] = []any{map[string]any{"descricao": "Mesa", "qtd": 2, "valor": 300}, map[string]any{"descricao": "Cadeira", "qtd": 4, "valor": 100}}
		saved, err := c.Insert(ped, SaveOpts{})
		if err != nil {
			return fmt.Errorf("linha 139: %w", err)
		}
		name = saved.Name()
		if !strings.HasPrefix(name, "PED-2026-") || saved.Str("cliente_tipo") != "PJ" || toFloat(saved["total"]) != 1000 {
			t.Fatalf("doc inesperado: %v", saved)
		}
		if len(saved.Children("itens")) != 2 || saved.Children("itens")[1].Str("descricao") != "Cadeira" {
			t.Fatalf("filhos: %v", saved["itens"])
		}
		// mandatoryDependsOn: total > 1000 exige desconto — validado no servidor
		saved["itens"] = append(anyList(saved.Children("itens")), map[string]any{"descricao": "Extra", "qtd": 1, "valor": 1})
		if _, err := c.Save(saved, SaveOpts{}); err == nil || cerr.From(err).Type != "MandatoryError" {
			t.Fatalf("esperava MandatoryError de desconto, veio %v", err)
		}
		saved, _ = c.GetDoc("Pedido", name)
		saved["itens"] = append(anyList(saved.Children("itens")), map[string]any{"descricao": "Extra", "qtd": 1, "valor": 1})
		saved["desconto"] = 5
		saved, err = c.Save(saved, SaveOpts{})
		if err != nil {
			return fmt.Errorf("linha 157: %w", err)
		}
		// versão gravada
		n, _ := c.Count("Version", map[string]any{"ref_doctype": "Pedido", "docname": name})
		if n != 1 {
			t.Fatalf("esperava 1 versão, veio %d", n)
		}
		// filtro por child
		rows, err := c.GetList("Pedido", ListArgs{Filters: []any{[]any{"Item Pedido", "descricao", "=", "Cadeira"}}, Fields: []string{"name", "total"}})
		if err != nil || len(rows) != 1 {
			t.Fatalf("filtro child: %v %v", rows, err)
		}
		// submit
		saved, err = c.Submit(saved)
		if err != nil {
			return fmt.Errorf("linha 172: %w", err)
		}
		lim, _ := c.GetValue("Pessoa", "Ana", "limite")
		if toFloat(lim) != 1001 {
			t.Fatalf("onSubmit não rodou: %v", lim)
		}
		saved["desconto"] = 10
		if _, err := c.Save(saved, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "cannot be changed after submission") {
			t.Fatalf("esperava bloqueio allowOnSubmit, veio %v", err)
		}
		saved, _ = c.GetDoc("Pedido", name)
		saved["obs"] = "ok"
		if _, err := c.Save(saved, SaveOpts{}); err != nil {
			t.Fatalf("obs é allowOnSubmit: %v", err)
		}
		// método
		rt, _ := c.RT()
		res, err := rt.RunMethod("Pedido", "resumo", saved.JSON(), []byte(`{"x":1}`))
		if err != nil || string(res.Result) != `{"itens":3,"total":1001,"x":1}` {
			t.Fatalf("método: %v %s", err, res.Result)
		}
		// delete bloqueado por link
		if err := c.Delete("Pessoa", "Ana", false, false); err == nil || cerr.From(err).Type != "LinkExistsError" {
			t.Fatalf("esperava LinkExistsError, veio %v", err)
		}
		// rename propaga
		if _, err := c.Rename("Pessoa", "Ana", "Ana Maria"); err != nil {
			return fmt.Errorf("linha 199: %w", err)
		}
		v, _ := c.GetValue("Pedido", name, "cliente")
		if v != "Ana Maria" {
			t.Fatalf("rename não propagou: %v", v)
		}
		// cancel + amend
		saved, _ = c.GetDoc("Pedido", name)
		if _, err := c.Cancel(saved); err != nil {
			return fmt.Errorf("linha 208: %w", err)
		}
		am, err := c.Amend("Pedido", name)
		if err != nil {
			return fmt.Errorf("linha 212: %w", err)
		}
		am, err = c.Insert(am, SaveOpts{})
		if err != nil {
			return fmt.Errorf("linha 217: %w", err)
		}
		if am.Name() != name+"-1" || am.Str("amended_from") != name || len(am.Children("itens")) != 3 {
			t.Fatalf("amend: %v", am)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// permissões: usuário sem papel não lê Pedido
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "ze@x.com", "full_name": "Zé"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "ze@x.com", func(c *Ctx) error {
		if _, err := c.GetDoc("Pedido", name); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("esperava PermissionError, veio %v", err)
		}
		rows, err := c.GetList("Pessoa", ListArgs{})
		if err != nil || len(rows) != 0 {
			t.Fatalf("ifOwner deveria esconder tudo: %v %v", rows, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// test runner em savepoints
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		rt, _ := c.RT()
		res, err := rt.RunTests("", "")
		if err != nil {
			return fmt.Errorf("linha 253: %w", err)
		}
		for _, r := range res {
			if !r.OK {
				t.Errorf("%s: %s", r.Name, r.Error)
			}
		}
		if len(res) != 2 {
			t.Fatalf("esperava 2 testes, veio %d", len(res))
		}
		n, _ := c.Count("Pessoa", map[string]any{"nome": "Teste"})
		if n != 0 {
			t.Fatalf("teste não foi revertido: %d", n)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func anyList(rows []Doc) []any {
	out := make([]any, len(rows))
	for i, r := range rows {
		out[i] = map[string]any(r)
	}
	return out
}
