package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

// permApp: Pedido (Gestor) com duas tabelas filhas, uma delas allowOnSubmit.
func permApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"] });`)
	w("doctypes/pessoa/pessoa.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pessoa", naming: { field: "nome" },
  fields: [{ fieldname: "nome", fieldtype: "Data", label: "Nome", reqd: true }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }, { role: "All", read: true }] });`)
	w("doctypes/item/item.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Item Pedido", isChild: true, fields: [
  { fieldname: "descricao", fieldtype: "Data", label: "Descrição", reqd: true } ] });`)
	w("doctypes/nota/nota.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Nota Pedido", isChild: true, fields: [
  { fieldname: "texto", fieldtype: "Data", label: "Texto" } ] });`)
	w("doctypes/pedido/pedido.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Pedido", naming: { series: "PED-.####" }, submittable: true,
  fields: [
    { fieldname: "cliente", fieldtype: "Link", label: "Cliente", options: "Pessoa", reqd: true },
    { fieldname: "itens", fieldtype: "Table", label: "Itens", options: "Item Pedido" },
    { fieldname: "notas", fieldtype: "Table", label: "Notas", options: "Nota Pedido", allowOnSubmit: true },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true, submit: true, cancel: true, amend: true }] });`)
	return dir
}

func setupPerm(t *testing.T) *Engine {
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
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "demo", Dir: permApp(t)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		for _, u := range []struct{ email, role string }{{"ana@x.com", "Gestor"}, {"ze@x.com", ""}} {
			d, _ := c.NewDoc("User", Doc{"email": u.email, "full_name": u.email})
			if u.role != "" {
				d["roles"] = []any{map[string]any{"role": u.role}}
			}
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		p, _ := c.NewDoc("Pessoa", Doc{"nome": "Cliente"})
		_, err := c.Insert(p, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestB02_ChildPermissionFollowsParent(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()

	// linha de Has Role do Administrator
	var roleRow string
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		u, _ := c.GetDoc("User", "Administrator")
		roleRow = u.Children("roles")[0].Name()
		return nil
	})
	if roleRow == "" {
		t.Fatal("Administrator sem Has Role")
	}

	for _, user := range []string{"Guest", "ze@x.com"} {
		err := e.Run(ctx, user, func(c *Ctx) error {
			if ok, _ := c.HasPermission("Has Role", "read", Doc{"name": roleRow}); ok {
				t.Errorf("%s: read em Has Role pelo nome deveria ser negado", user)
			}
			if ok, _ := c.HasPermission("Has Role", "read", Doc{"parenttype": "User", "parent": "Administrator", "parentfield": "roles"}); ok {
				t.Errorf("%s: read em Has Role com pai deveria ser negado", user)
			}
			if ok, _ := c.HasPermission("Has Role", "read", nil); ok {
				t.Errorf("%s: read em Has Role (doctype) deveria ser negado", user)
			}
			if _, err := c.GetDoc("Has Role", roleRow); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: GetDoc Has Role: esperava PermissionError, veio %v", user, err)
			}
			if _, err := c.GetList("Has Role", ListArgs{}); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: GetList Has Role: esperava PermissionError, veio %v", user, err)
			}
			row, _ := c.GetDocIgnoringPerms("Has Role", roleRow)
			row["role"] = "Guest"
			if _, err := c.Save(row, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: Save Has Role: esperava PermissionError, veio %v", user, err)
			}
			if err := c.Delete("Has Role", roleRow, false, false); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: Delete Has Role: esperava PermissionError, veio %v", user, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// nada mudou
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		v, _ := c.GetValue("Has Role", roleRow, "role")
		if v != "System Manager" {
			t.Fatalf("linha alterada: %v", v)
		}
		return nil
	})

	// Gestor: pedido em rascunho → escreve nos filhos; após submit só no allowOnSubmit
	var item, nota, ped string
	err := e.Run(ctx, "ana@x.com", func(c *Ctx) error {
		p, _ := c.NewDoc("Pedido", Doc{"cliente": "Cliente"})
		p["itens"] = []any{map[string]any{"descricao": "a"}}
		p["notas"] = []any{map[string]any{"texto": "n"}}
		saved, err := c.Insert(p, SaveOpts{})
		if err != nil {
			return err
		}
		ped, item, nota = saved.Name(), saved.Children("itens")[0].Name(), saved.Children("notas")[0].Name()
		for _, dt := range []string{"Item Pedido", "Nota Pedido"} {
			for _, pt := range []string{"read", "write", "delete"} {
				n := item
				if dt == "Nota Pedido" {
					n = nota
				}
				if ok, err := c.HasPermission(dt, pt, Doc{"name": n}); !ok || err != nil {
					t.Errorf("rascunho: %s %s deveria ser permitido: %v", pt, dt, err)
				}
			}
		}
		rows, err := c.GetList("Item Pedido", ListArgs{Fields: []string{"name", "parenttype"}})
		if err != nil || len(rows) != 1 {
			t.Errorf("GetList Item Pedido como Gestor: %v %v", rows, err)
		}
		if _, err := c.Submit(saved); err != nil {
			return err
		}
		if ok, _ := c.HasPermission("Item Pedido", "write", Doc{"name": item}); ok {
			t.Errorf("enviado: write em Item Pedido deveria ser negado")
		}
		if ok, _ := c.HasPermission("Nota Pedido", "write", Doc{"name": nota}); !ok {
			t.Errorf("enviado: write em Nota Pedido (allowOnSubmit) deveria ser permitido")
		}
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"name": item}); !ok {
			t.Errorf("enviado: read em Item Pedido deveria ser permitido")
		}
		doc, _ := c.GetDoc("Pedido", ped)
		if _, err := c.Cancel(doc); err != nil {
			return err
		}
		if ok, _ := c.HasPermission("Nota Pedido", "write", Doc{"name": nota}); ok {
			t.Errorf("cancelado: write em Nota Pedido deveria ser negado")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// usuário sem papel não lê filhos de Pedido nem lista Item Pedido
	e.Run(ctx, "ze@x.com", func(c *Ctx) error {
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"name": item}); ok {
			t.Errorf("zé: read em Item Pedido deveria ser negado")
		}
		if _, err := c.GetList("Item Pedido", ListArgs{}); err == nil {
			t.Errorf("zé: GetList Item Pedido deveria falhar")
		}
		return nil
	})
	// linha órfã / pai inexistente → negado
	e.Run(ctx, "ana@x.com", func(c *Ctx) error {
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"parenttype": "Pedido", "parent": "NAO-EXISTE", "parentfield": "itens"}); ok {
			t.Errorf("pai inexistente deveria ser negado")
		}
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"parenttype": "Pedido", "parent": ped, "parentfield": "notas"}); ok {
			t.Errorf("parentfield que não usa o child deveria ser negado")
		}
		return nil
	})
}
