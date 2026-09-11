package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// Dois apps: `loja` é dono de Produto; `addons` estende. É a forma que a
// migração de um app Frappe assume aqui — os Custom Fields não moram no app
// que declarou o DocType.
func extendApps(t *testing.T) []js.App {
	write := func(dir string) func(rel, src string) {
		return func(rel, src string) {
			os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
			os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
		}
	}

	host := t.TempDir()
	w := write(host)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "loja", title: "Loja", roles: ["Vendedor"] });`)
	w("doctypes/produto/produto.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Produto", naming: { field: "codigo" }, label: "Produto",
  fields: [
    { fieldname: "codigo", fieldtype: "Data", label: "Code", reqd: true },
    { fieldname: "nome", fieldtype: "Data", label: "Name" },
  ],
  permissions: [{ role: "Vendedor", read: true, write: true, create: true, delete: true, report: true }] });`)
	w("doctypes/produto/produto.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Produto", {
  // o dono libera; quem estende ainda pode negar
  hasPermission() { return true; },
  permissionQuery() { return { nome: ["!=", "oculto"] }; },
});`)
	w("doctypes/produto/produto.form.ts", `import { defineForm } from "@ddcore/desk-sdk";
defineForm("Produto", { refresh() {} });`)

	addons := t.TempDir()
	w = write(addons)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "addons", title: "Addons", requires: ["loja"] });`)
	w("extensions/produto.extend.ts", `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("Produto", {
  fields: [{ fieldname: "garantia_meses", fieldtype: "Int", label: "Warranty (months)", insertAfter: "codigo" }],
  set: { nome: { label: "Product name", reqd: true } },
  doctype: { trackChanges: true, titleField: "nome" },
  permissions: [{ role: "Auditor", read: true, report: true }],
  hasPermission(doc: any, ptype: string) { if (ptype === "delete" && doc && doc.codigo === "P-1") return false; },
  permissionQuery() { return { codigo: ["!=", "P-9"] }; },
});`)
	w("extensions/produto.form.ts", `import { defineForm } from "@ddcore/desk-sdk";
defineForm("Produto", { refresh() {} });`)

	return []js.App{{Name: "loja", Dir: host}, {Name: "addons", Dir: addons}}
}

func setupExtend(t *testing.T) *Engine {
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
	e, err := New(ctx, Config{DSN: testDSN, Apps: extendApps(t), Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	return e
}

// O campo que outro app acrescentou é um campo como qualquer outro: vira
// coluna, valida, grava e volta na leitura.
func TestExtensionFieldBecomesAColumn(t *testing.T) {
	e := setupExtend(t)
	ctx := context.Background()

	d, ok := e.Current().Meta.Get("Produto")
	if !ok {
		t.Fatal("Produto não carregou")
	}
	f := d.Field("garantia_meses")
	if f == nil || f.App != "addons" {
		t.Fatalf("campo da extensão: %+v", f)
	}
	if got := d.Fields[1].Fieldname; got != "garantia_meses" {
		t.Fatalf("insertAfter não respeitado, índice 1 = %q", got)
	}
	if got := d.Field("nome"); got.Label != "Product name" || !got.Reqd {
		t.Fatalf("property setter não aplicado: %+v", got)
	}
	if !d.TrackChanges || d.TitleField != "nome" {
		t.Fatalf("property setter de DocType não aplicado: trackChanges=%v titleField=%q", d.TrackChanges, d.TitleField)
	}
	if got := d.ExtendedBy; len(got) != 1 || got[0] != "addons" {
		t.Fatalf("ExtendedBy=%v", got)
	}
	// o script de formulário do dono e o de quem estende, nessa ordem
	if got := d.FormApps; len(got) != 2 || got[0] != "loja" || got[1] != "addons" {
		t.Fatalf("FormApps=%v", got)
	}

	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.NewDoc("Produto", Doc{"codigo": "P-1", "nome": "Cadeira", "garantia_meses": 24})
		if err != nil {
			return err
		}
		if _, err := c.Insert(doc, SaveOpts{}); err != nil {
			return err
		}
		back, err := c.GetDoc("Produto", "P-1")
		if err != nil {
			return err
		}
		if v := toFloat(back["garantia_meses"]); v != 24 {
			t.Fatalf("garantia_meses=%v", v)
		}
		// e o reqd que a extensão impôs vale no servidor
		vazio, _ := c.NewDoc("Produto", Doc{"codigo": "P-2"})
		if _, err := c.Insert(vazio, SaveOpts{}); err == nil {
			t.Fatal("esperava erro de campo obrigatório imposto pela extensão")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A meta que o TS enxerga é a mesma que o banco e o desk enxergam: o app não
// pode decidir por um DocType que outro app já estendeu.
func TestExtensionIsVisibleFromTypeScript(t *testing.T) {
	e := setupExtend(t)
	out, _, err := e.Eval(context.Background(), `ddcore.getMeta("Produto").fields.map((f: any) => f.fieldname).join(",")`, false)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	json.Unmarshal(out, &got)
	if !strings.Contains(got, "garantia_meses") {
		t.Fatalf("getMeta não trouxe o campo da extensão: %q", got)
	}
}

// Permissões: a extensão só acrescenta papéis, e a negação dela vence o
// "pode" do dono.
func TestExtensionPermissionsChain(t *testing.T) {
	e := setupExtend(t)
	ctx := context.Background()

	d, _ := e.Current().Meta.Get("Produto")
	if !hasPerm(d.Permissions, "Auditor") {
		t.Fatal("papel da extensão não entrou nas permissões")
	}

	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "ana@x.com", "full_name": "Ana"})
		u["roles"] = []any{map[string]any{"role": "Vendedor"}}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, cod := range []string{"P-1", "P-2", "P-9"} {
			doc, _ := c.NewDoc("Produto", Doc{"codigo": cod, "nome": cod})
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = e.Run(ctx, "ana@x.com", func(c *Ctx) error {
		p1, err := c.GetDoc("Produto", "P-1")
		if err != nil {
			return err
		}
		ok, err := c.HasPermission("Produto", "delete", p1)
		if err != nil {
			return err
		}
		if ok {
			t.Fatal("a extensão negou o delete de P-1 e o dono não pode desfazer isso")
		}
		p2, err := c.GetDoc("Produto", "P-2")
		if err != nil {
			return err
		}
		if ok, err = c.HasPermission("Produto", "delete", p2); err != nil || !ok {
			t.Fatalf("P-2 deveria seguir permitido: %v %v", ok, err)
		}
		// os dois permissionQuery se somam: o do dono esconde "oculto", o da
		// extensão esconde P-9
		rows, err := c.GetList("Produto", ListArgs{Limit: 50})
		if err != nil {
			return err
		}
		for _, r := range rows {
			if db.Str(r["codigo"]) == "P-9" {
				t.Fatal("permissionQuery da extensão não foi aplicado")
			}
		}
		if len(rows) != 2 {
			t.Fatalf("esperava 2 produtos visíveis, veio %d", len(rows))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Os erros de carga não precisam de banco: são meta. Um conflito tem que
// impedir a carga inteira, nomeando os dois apps — o desenvolvedor precisa
// saber com quem está discutindo.
func TestExtensionConflictsRefuseToLoad(t *testing.T) {
	base := extendApps(t)
	write := func(dir string) func(rel, src string) {
		return func(rel, src string) {
			os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
			os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
		}
	}

	// um terceiro app que sobrescreve o mesmo label que `addons` já sobrescreve
	rival := t.TempDir()
	w := write(rival)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rival", title: "Rival", requires: ["loja"] });`)
	w("extensions/produto.extend.ts", `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("Produto", { set: { nome: { label: "Descrição" } } });`)

	apps := append(append([]js.App{}, base...), js.App{Name: "rival", Dir: rival})
	_, err := New(context.Background(), Config{Apps: apps})
	if err == nil {
		t.Fatal("dois apps sobrescrevendo a mesma propriedade deveriam impedir a carga")
	}
	for _, want := range []string{"addons", "rival", `"label"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("o erro não menciona %s: %v", want, err)
		}
	}

	// e um controller para o DocType de outro app é recusado na hora do registro
	intruso := t.TempDir()
	w = write(intruso)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "intruso", title: "Intruso", requires: ["loja"] });`)
	w("doctypes/produto/produto.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Produto", { validate() {} });`)

	apps = append(append([]js.App{}, base...), js.App{Name: "intruso", Dir: intruso})
	_, err = New(context.Background(), Config{Apps: apps})
	if err == nil {
		t.Fatal("um segundo controller substituiria o do dono em silêncio")
	}
	if !strings.Contains(err.Error(), "already has a controller") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func hasPerm(perms []meta.Perm, role string) bool {
	for _, p := range perms {
		if p.Role == role {
			return true
		}
	}
	return false
}
