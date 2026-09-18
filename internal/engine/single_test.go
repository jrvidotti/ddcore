package engine

import (
	"context"
	"errors"
	"github.com/jrvidotti/ddcore/internal/cerr"
	"os"
	"path/filepath"
	"testing"
)

func TestSingleDefaultsAndPersistence(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Settings", isSingle: true, fields: [
 {fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true},
 {fieldname: "items", fieldtype: "Table", label: "Items", options: "Item Pedido"}
], permissions: [{role: "All", read: true, write: true}]});`})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		if doc.Name() != "singleton" || doc["__islocal"] != true || doc["enabled"] != true {
			t.Fatalf("defaults: %v", doc)
		}
		doc["enabled"] = false
		doc["items"] = []any{map[string]any{"descricao": "First", "qtd": 2}}
		saved, err := c.Save(doc, SaveOpts{})
		if err != nil {
			return err
		}
		if saved.Name() != "singleton" || saved["enabled"] != false || len(saved.Children("items")) != 1 {
			t.Fatalf("saved: %v", saved)
		}
		again, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		if again["enabled"] != false {
			t.Fatalf("default replaced saved false: %v", again)
		}
		if _, err := c.GetDoc("Settings", "other"); err == nil {
			t.Fatal("alternate identity accepted")
		}
		if err := c.Delete("Settings", "singleton", true, true); err == nil {
			t.Fatal("single deleted")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSingleFirstSaveConcurrency(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name: "Settings", isSingle: true, fields: [{fieldname:"value", fieldtype:"Data", label:"Value"}]});`})
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			results <- e.Run(context.Background(), "Admin", func(c *Ctx) error {
				doc, err := c.GetDoc("Settings", "")
				if err != nil {
					ready <- struct{}{}
					return err
				}
				ready <- struct{}{}
				<-release
				_, err = c.Save(doc, SaveOpts{})
				return err
			})
		}()
	}
	<-ready
	<-ready
	close(release)
	a, b := <-results, <-results
	if (a == nil) == (b == nil) {
		t.Fatalf("expected one success and one conflict: %v, %v", a, b)
	}
}

func TestSinglePermissionsRollbackAndSDK(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name: "Settings", isSingle: true, trackChanges: true, fields: [{fieldname:"value", fieldtype:"Data", label:"Value", default:"initial"}], permissions:[{role:"All", read:true, create:true},{role:"Gestor", read:true, write:true}]});`})
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, name := range []string{"reader@example.com", "writer@example.com"} {
			u, err := c.NewDoc("User", Doc{"email": name, "full_name": name})
			if err != nil {
				return err
			}
			if name == "writer@example.com" {
				u["roles"] = []any{map[string]any{"role": "Gestor"}}
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "reader@example.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		_, err = c.Save(doc, SaveOpts{})
		return err
	})
	if err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("create bypassed write: %v", err)
	}
	rollback := errors.New("rollback")
	err = e.Run(ctx, "writer@example.com", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("rollback: %v", err)
	}
	out, _, err := e.Eval(ctx, `const doc = ddcore.getDoc("Settings"); if (!doc.isNew()) throw new Error("rollback persisted"); doc.value = "saved"; doc.save(); doc.reload(); doc.value;`, false)
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != `"saved"` {
		t.Fatalf("SDK reload: %v", out)
	}
}

func TestSingleDatabaseInvariantAndMigration(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name: "Settings", isSingle: true, fields: []});`})
	ctx := context.Background()
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO tab_settings(name) VALUES ('other')`); err == nil {
		t.Fatal("database accepted alternate identity")
	}
	if _, err := e.DB.Pool.Exec(ctx, `INSERT INTO tab_settings(name,docstatus) VALUES ('singleton',1)`); err == nil {
		t.Fatal("database accepted submitted Single")
	}
	d, _ := e.DocType("Settings")
	d.IsSingle = false
	if _, err := e.Plan(ctx, false); err == nil {
		t.Fatal("conversion accepted")
	}
	d.IsSingle = true
}

func TestSingleUpdateHooksAndConflict(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/settings/settings.doctype.ts":    `import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name:"Settings", isSingle:true, trackChanges:true, fields:[{fieldname:"value",fieldtype:"Int",label:"Value",default:0},{fieldname:"inserts",fieldtype:"Int",label:"Inserts",default:0}]});`,
		"doctypes/settings/settings.controller.ts": `import { defineController } from "@ddcore/sdk"; export default defineController("Settings", { beforeInsert(doc) { doc.inserts += 1; }, validate(doc) { if (doc.value < 0) ddcore.throw("Value cannot be negative"); } });`,
	})
	ctx := context.Background()
	var stale Doc
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		stale, err = c.Save(doc, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		doc["value"] = 1
		doc, err = c.Save(doc, SaveOpts{})
		if err != nil {
			return err
		}
		if toFloat(doc["inserts"]) != 1 {
			t.Fatalf("insert hook ran on update: %v", doc)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error { stale["value"] = 2; _, err := c.Save(stale, SaveOpts{}); return err })
	if err == nil || cerr.From(err).Type != "TimestampMismatchError" {
		t.Fatalf("stale update: %v", err)
	}
	err = e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		doc["value"] = -1
		_, err = c.Save(doc, SaveOpts{})
		return err
	})
	if err == nil {
		t.Fatal("validation bypassed")
	}
}

func TestSingleDeclaredRenames(t *testing.T) {
	e := setupWith(t, map[string]string{"doctypes/settings/settings.doctype.ts": `import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name: "Settings", isSingle: true, fields: [{fieldname:"value",fieldtype:"Data",label:"Value"}]});`})
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Settings", "")
		if err != nil {
			return err
		}
		doc["value"] = "retained"
		_, err = c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(e.App("demo").Dir, "doctypes/settings/settings.doctype.ts")
	if err := os.WriteFile(source, []byte(`import { defineDoctype } from "@ddcore/sdk"; export default defineDoctype({name: "Renamed Settings", renamedFrom:["Settings"], isSingle: true, fields: [{fieldname:"renamed_value",renamedFrom:["value"],fieldtype:"Data",label:"Value"},{fieldname:"enabled",fieldtype:"Check",label:"Enabled"}]});`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.Load(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	if plan, err := e.Plan(ctx, false); err != nil || len(plan) != 0 {
		t.Fatalf("migration not idempotent: %v %v", plan, err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, err := c.GetDoc("Renamed Settings", "")
		if err != nil {
			return err
		}
		if doc.Str("renamed_value") != "retained" || doc.Name() != "singleton" {
			t.Fatalf("rename lost settings: %v", doc)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
