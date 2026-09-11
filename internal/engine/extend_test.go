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

// Two apps: `loja` owns Produto; `addons` extends it. This is the form that
// Frappe app migration assumes here — Custom Fields do not reside in the app
// that declared the DocType.
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
  // owner permits; extender can still deny
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
			t.Fatalf("postgres unavailable on DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
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

// The field added by another app is a field like any other: becomes
// a column, validates, saves, and returns on read.
func TestExtensionFieldBecomesAColumn(t *testing.T) {
	e := setupExtend(t)
	ctx := context.Background()

	d, ok := e.Current().Meta.Get("Produto")
	if !ok {
		t.Fatal("Produto did not load")
	}
	f := d.Field("garantia_meses")
	if f == nil || f.App != "addons" {
		t.Fatalf("extension field: %+v", f)
	}
	if got := d.Fields[1].Fieldname; got != "garantia_meses" {
		t.Fatalf("insertAfter not respected, index 1 = %q", got)
	}
	if got := d.Field("nome"); got.Label != "Product name" || !got.Reqd {
		t.Fatalf("property setter not applied: %+v", got)
	}
	if !d.TrackChanges || d.TitleField != "nome" {
		t.Fatalf("DocType property setter not applied: trackChanges=%v titleField=%q", d.TrackChanges, d.TitleField)
	}
	if got := d.ExtendedBy; len(got) != 1 || got[0] != "addons" {
		t.Fatalf("ExtendedBy=%v", got)
	}
	// owner form script and extender form script, in that order
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
		// and the reqd imposed by the extension applies on the server
		vazio, _ := c.NewDoc("Produto", Doc{"codigo": "P-2"})
		if _, err := c.Insert(vazio, SaveOpts{}); err == nil {
			t.Fatal("expected mandatory field error imposed by extension")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Metadata seen by TS is identical to what database and desk see: the app cannot
// decide for a DocType that another app has already extended.
func TestExtensionIsVisibleFromTypeScript(t *testing.T) {
	e := setupExtend(t)
	out, _, err := e.Eval(context.Background(), `ddcore.getMeta("Produto").fields.map((f: any) => f.fieldname).join(",")`, false)
	if err != nil {
		t.Fatal(err)
	}
	var got string
	json.Unmarshal(out, &got)
	if !strings.Contains(got, "garantia_meses") {
		t.Fatalf("getMeta did not return extension field: %q", got)
	}
}

// Permissions: extension only adds roles, and its denial overrides owner permission.
func TestExtensionPermissionsChain(t *testing.T) {
	e := setupExtend(t)
	ctx := context.Background()

	d, _ := e.Current().Meta.Get("Produto")
	if !hasPerm(d.Permissions, "Auditor") {
		t.Fatal("extension role was not added to permissions")
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
			t.Fatal("extension denied deleting P-1 and owner cannot override it")
		}
		p2, err := c.GetDoc("Produto", "P-2")
		if err != nil {
			return err
		}
		if ok, err = c.HasPermission("Produto", "delete", p2); err != nil || !ok {
			t.Fatalf("P-2 should remain permitted: %v %v", ok, err)
		}
		// both permissionQueries combine: owner hides "oculto", extension hides P-9
		rows, err := c.GetList("Produto", ListArgs{Limit: 50})
		if err != nil {
			return err
		}
		for _, r := range rows {
			if db.Str(r["codigo"]) == "P-9" {
				t.Fatal("extension permissionQuery was not applied")
			}
		}
		if len(rows) != 2 {
			t.Fatalf("expected 2 visible products, got %d", len(rows))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// Load errors do not need database: they are meta. A conflict must
// block the entire load, naming both apps — the developer needs
// to know what is conflicting.
func TestExtensionConflictsRefuseToLoad(t *testing.T) {
	base := extendApps(t)
	write := func(dir string) func(rel, src string) {
		return func(rel, src string) {
			os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
			os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
		}
	}

	// a third app overriding the same label that `addons` already overrides
	rival := t.TempDir()
	w := write(rival)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "rival", title: "Rival", requires: ["loja"] });`)
	w("extensions/produto.extend.ts", `import { extendDoctype } from "@ddcore/sdk";
export default extendDoctype("Produto", { set: { nome: { label: "Descrição" } } });`)

	apps := append(append([]js.App{}, base...), js.App{Name: "rival", Dir: rival})
	_, err := New(context.Background(), Config{Apps: apps})
	if err == nil {
		t.Fatal("two apps overriding the same property should block load")
	}
	for _, want := range []string{"addons", "rival", `"label"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error does not mention %s: %v", want, err)
		}
	}

	// and a controller for another app's DocType is rejected at registration
	intruso := t.TempDir()
	w = write(intruso)
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "intruso", title: "Intruso", requires: ["loja"] });`)
	w("doctypes/produto/produto.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Produto", { validate() {} });`)

	apps = append(append([]js.App{}, base...), js.App{Name: "intruso", Dir: intruso})
	_, err = New(context.Background(), Config{Apps: apps})
	if err == nil {
		t.Fatal("a second controller would silently replace the owner controller")
	}
	if !strings.Contains(err.Error(), "already has a controller") {
		t.Fatalf("unexpected error: %v", err)
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
