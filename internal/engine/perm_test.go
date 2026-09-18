package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

func TestUserPermissionDocTypeLoaded(t *testing.T) {
	e := setupPerm(t)
	dt, ok := e.Meta.Get("User Permission")
	if !ok {
		t.Fatalf("expected User Permission DocType to be loaded")
	}
	if dt.TableName() != "tab_user_permission" {
		t.Fatalf("expected table name tab_user_permission, got %s", dt.TableName())
	}
	if f := dt.Field("for_value"); f == nil || !f.Reqd {
		t.Fatalf("expected for_value field to be required")
	}
}

func TestUserPermissionsResolutionAndCacheInvalidation(t *testing.T) {
	e := setupPerm(t)
	ctx := context.Background()
	const (
		user    = "ana@x.com"
		newUser = "ze@x.com"
	)

	resolve := func(target string, want int, wantValue string) {
		t.Helper()
		if err := e.Run(ctx, target, func(c *Ctx) error {
			perms, err := c.UserPermissions()
			if err != nil {
				return err
			}
			cached, err := c.UserPermissions()
			if err != nil {
				return err
			}
			if c.userPerms == nil || len(cached) != len(perms) {
				t.Fatalf("expected UserPermissions to retain its request-local cache")
			}
			if len(perms) != want {
				t.Fatalf("expected %d user permissions, got %d", want, len(perms))
			}
			if want > 0 && (perms[0].User != target || perms[0].Allow != "Company" || perms[0].ForValue != wantValue || !perms[0].IsDefault) {
				t.Fatalf("unexpected user permission: %+v", perms[0])
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	// Prewarm an empty result so the insert must invalidate the shared cache.
	resolve(user, 0, "")

	var name string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.Insert(Doc{
			"doctype":    "User Permission",
			"user":       user,
			"allow":      "Company",
			"for_value":  "Acme Corp",
			"is_default": true,
		}, SaveOpts{})
		if err == nil {
			name = doc.Name()
		}
		return err
	}); err != nil {
		t.Fatalf("insert user permission: %v", err)
	}
	resolve(user, 1, "Acme Corp")

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("User Permission", name)
		if err != nil {
			return err
		}
		doc["for_value"] = "Globex Corp"
		_, err = c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("update user permission: %v", err)
	}
	resolve(user, 1, "Globex Corp")
	resolve(newUser, 0, "")

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.DBSet("User Permission", name, Doc{"user": newUser, "for_value": "Initech"}, true)
		return err
	}); err != nil {
		t.Fatalf("DBSet user permission: %v", err)
	}
	resolve(user, 0, "")
	resolve(newUser, 1, "Initech")

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		return c.Delete("User Permission", name, false, false)
	}); err != nil {
		t.Fatalf("delete user permission: %v", err)
	}
	resolve(newUser, 0, "")

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		perms, err := c.UserPermissions()
		if err != nil {
			return err
		}
		if len(perms) != 0 {
			t.Fatalf("expected Admin permissions to be bypassed, got %d", len(perms))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, user, func(c *Ctx) error {
		return c.WithIgnorePermissions(func() error {
			perms, err := c.UserPermissions()
			if err != nil {
				return err
			}
			if len(perms) != 0 {
				t.Fatalf("expected ignored permissions to be bypassed, got %d", len(perms))
			}
			return nil
		})
	}); err != nil {
		t.Fatal(err)
	}
}

// permApp: Pedido (Gestor) with two child tables, one having allowOnSubmit.
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
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
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

	// Admin's Has Role row
	var roleRow string
	e.Run(ctx, "Admin", func(c *Ctx) error {
		u, _ := c.GetDoc("User", "Admin")
		roleRow = u.Children("roles")[0].Name()
		return nil
	})
	if roleRow == "" {
		t.Fatal("Admin has no Has Role")
	}

	for _, user := range []string{"Guest", "ze@x.com"} {
		err := e.Run(ctx, user, func(c *Ctx) error {
			if ok, _ := c.HasPermission("Has Role", "read", Doc{"name": roleRow}); ok {
				t.Errorf("%s: read on Has Role by name should be denied", user)
			}
			if ok, _ := c.HasPermission("Has Role", "read", Doc{"parenttype": "User", "parent": "Admin", "parentfield": "roles"}); ok {
				t.Errorf("%s: read on Has Role with parent should be denied", user)
			}
			if ok, _ := c.HasPermission("Has Role", "read", nil); ok {
				t.Errorf("%s: read on Has Role (doctype) should be denied", user)
			}
			if _, err := c.GetDoc("Has Role", roleRow); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: GetDoc Has Role: expected PermissionError, got %v", user, err)
			}
			if _, err := c.GetList("Has Role", ListArgs{}); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: GetList Has Role: expected PermissionError, got %v", user, err)
			}
			row, _ := c.GetDocIgnoringPerms("Has Role", roleRow)
			row["role"] = "Guest"
			if _, err := c.Save(row, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: Save Has Role: expected PermissionError, got %v", user, err)
			}
			if err := c.Delete("Has Role", roleRow, false, false); err == nil || cerr.From(err).Type != "PermissionError" {
				t.Errorf("%s: Delete Has Role: expected PermissionError, got %v", user, err)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// nothing changed
	e.Run(ctx, "Admin", func(c *Ctx) error {
		v, _ := c.GetValue("Has Role", roleRow, "role")
		if v != "System Manager" {
			t.Fatalf("row modified: %v", v)
		}
		return nil
	})

	// Gestor: draft pedido -> writes to children; after submit only allowOnSubmit
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
					t.Errorf("draft: %s %s should be permitted: %v", pt, dt, err)
				}
			}
		}
		rows, err := c.GetList("Item Pedido", ListArgs{Fields: []string{"name", "parenttype"}})
		if err != nil || len(rows) != 1 {
			t.Errorf("GetList Item Pedido as Gestor: %v %v", rows, err)
		}
		if _, err := c.Submit(saved); err != nil {
			return err
		}
		if ok, _ := c.HasPermission("Item Pedido", "write", Doc{"name": item}); ok {
			t.Errorf("submitted: write on Item Pedido should be denied")
		}
		if ok, _ := c.HasPermission("Nota Pedido", "write", Doc{"name": nota}); !ok {
			t.Errorf("submitted: write on Nota Pedido (allowOnSubmit) should be permitted")
		}
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"name": item}); !ok {
			t.Errorf("submitted: read on Item Pedido should be permitted")
		}
		doc, _ := c.GetDoc("Pedido", ped)
		if _, err := c.Cancel(doc); err != nil {
			return err
		}
		if ok, _ := c.HasPermission("Nota Pedido", "write", Doc{"name": nota}); ok {
			t.Errorf("cancelled: write on Nota Pedido should be denied")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// user without role does not read Pedido children nor list Item Pedido
	e.Run(ctx, "ze@x.com", func(c *Ctx) error {
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"name": item}); ok {
			t.Errorf("ze: read on Item Pedido should be denied")
		}
		if _, err := c.GetList("Item Pedido", ListArgs{}); err == nil {
			t.Errorf("ze: GetList Item Pedido should fail")
		}
		return nil
	})
	// orphan row / nonexistent parent -> denied
	e.Run(ctx, "ana@x.com", func(c *Ctx) error {
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"parenttype": "Pedido", "parent": "NAO-EXISTE", "parentfield": "itens"}); ok {
			t.Errorf("nonexistent parent should be denied")
		}
		if ok, _ := c.HasPermission("Item Pedido", "read", Doc{"parenttype": "Pedido", "parent": ped, "parentfield": "notas"}); ok {
			t.Errorf("parentfield not using child should be denied")
		}
		return nil
	})
}
