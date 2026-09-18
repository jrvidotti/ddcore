package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

func sec01App(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	write := func(rel, src string) {
		t.Helper()
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "scope_test", title: "Scope Test", roles: ["Scope User"] });`)
	write("doctypes/test_company/test_company.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Company", naming: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scope User", read: true, create: true }] });`)
	write("doctypes/test_division/test_division.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Division", naming: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scope User", read: true }] });`)
	write("doctypes/test_record/test_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record", naming: { field: "title" }, submittable: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Test Company" },
    { fieldname: "division", fieldtype: "Link", label: "Division", options: "Test Division" },
    { fieldname: "items", fieldtype: "Table", label: "Items", options: "Test Record Item" },
  ],
  permissions: [{ role: "Scope User", read: true, create: true, write: true, delete: true, submit: true }] });`)
	write("doctypes/test_record/test_record.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Test Record", {
  beforeSave(doc) {
    if (doc.title === "Hook Insert Beta" || doc.title === "Hook Save Beta") doc.company = "Beta";
  },
  beforeSubmit(doc) {
    if (doc.title === "Hook Submit Beta") doc.company = "Beta";
  },
});`)
	write("doctypes/test_record_item/test_record_item.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record Item", isChild: true,
  fields: [{ fieldname: "company", fieldtype: "Link", label: "Company", options: "Test Company" }] });`)
	write("doctypes/test_dynamic_record/test_dynamic_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Dynamic Record", naming: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "party_type", fieldtype: "Data", label: "Party Type" },
    { fieldname: "party_name", fieldtype: "Dynamic Link", label: "Party Name", options: "party_type" },
  ],
  permissions: [{ role: "Scope User", read: true }] });`)
	return dir
}

func setupSEC01(t *testing.T) *Engine {
	t.Helper()
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	if _, err := e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName); err != nil {
		e0.DB.Close()
		t.Fatal(err)
	}
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		e0.DB.Close()
		t.Fatal(err)
	}
	e0.DB.Close()

	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "scope_test", Dir: sec01App(t)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		e.DB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	return e
}

func TestSEC01_QueryFilters(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const (
		alfaUser = "user_alfa@x.com"
		betaUser = "user_beta@x.com"
	)
	var alfaRecord, betaRecord, alfaDynamic, userDynamic string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, user := range []string{alfaUser, betaUser} {
			u, err := c.NewDoc("User", Doc{
				"email": user, "full_name": user,
				"roles": []any{map[string]any{"role": "Scope User"}},
			})
			if err != nil {
				return err
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa Division", "Beta Division"} {
			doc, err := c.NewDoc("Test Division", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, record := range []struct{ title, company, division string }{
			{"Alfa Record", "Alfa", "Alfa Division"},
			{"Alfa Wrong Division", "Alfa", "Beta Division"},
			{"Beta Record", "Beta", "Beta Division"},
			{"Unassigned Record", "", ""},
		} {
			doc, err := c.NewDoc("Test Record", Doc{"title": record.title, "company": record.company, "division": record.division})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			if record.company == "Alfa" {
				if record.division == "Alfa Division" {
					alfaRecord = saved.Name()
				}
			}
			if record.company == "Beta" {
				betaRecord = saved.Name()
			}
		}
		for _, record := range []struct{ title, partyType, partyName string }{
			{"Alfa Dynamic", "Test Company", "Alfa"},
			{"Beta Dynamic", "Test Company", "Beta"},
			{"User Dynamic", "User", alfaUser},
		} {
			doc, err := c.NewDoc("Test Dynamic Record", Doc{"title": record.title, "party_type": record.partyType, "party_name": record.partyName})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			if record.title == "Alfa Dynamic" {
				alfaDynamic = saved.Name()
			}
			if record.title == "User Dynamic" {
				userDynamic = saved.Name()
			}
		}
		for _, permission := range []Doc{
			{"user": alfaUser, "allow": "Test Company", "for_value": "Alfa"},
			{"user": alfaUser, "allow": "Test Division", "for_value": "Alfa Division", "applicable_for": "Test Record"},
			{"user": betaUser, "allow": "Test Company", "for_value": "Beta"},
			{"user": betaUser, "allow": "Test Division", "for_value": "Beta Division", "applicable_for": "Test Record"},
		} {
			doc, err := c.NewDoc("User Permission", permission)
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, alfaUser, func(c *Ctx) error {
		rows, err := c.GetList("Test Record", ListArgs{Fields: []string{"name", "company"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["name"] != alfaRecord || rows[0]["company"] != "Alfa" {
			t.Fatalf("GetList leaked records outside Alfa: %#v", rows)
		}

		count, err := c.Count("Test Record", nil)
		if err != nil {
			return err
		}
		if count != 1 {
			t.Fatalf("Count leaked records outside Alfa: got %d, want 1", count)
		}

		links, err := c.LinkSearch("Test Record", "", nil, 20)
		if err != nil {
			return err
		}
		if len(links) != 1 || links[0]["name"] != alfaRecord {
			t.Fatalf("LinkSearch leaked records outside Alfa: %#v", links)
		}

		companies, err := c.GetList("Test Company", ListArgs{Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(companies) != 1 || companies[0]["name"] != "Alfa" {
			t.Fatalf("direct entity query ignored Alfa scope: %#v", companies)
		}

		divisions, err := c.GetList("Test Division", ListArgs{Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(divisions) != 2 {
			t.Fatalf("applicable_for scope leaked outside Test Record: %#v", divisions)
		}

		dynamic, err := c.GetList("Test Dynamic Record", ListArgs{Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(dynamic) != 2 || !hasName(dynamic, alfaDynamic) || !hasName(dynamic, userDynamic) {
			t.Fatalf("Dynamic Link scope leaked a restricted party: %#v", dynamic)
		}

		ignored, err := c.GetList("Test Record", ListArgs{Fields: []string{"name"}, IgnorePermissions: true})
		if err != nil {
			return err
		}
		if len(ignored) != 1 || ignored[0]["name"] != alfaRecord {
			t.Fatalf("ListArgs.IgnorePermissions bypassed Alfa scope: %#v", ignored)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, betaUser, func(c *Ctx) error {
		rows, err := c.GetList("Test Record", ListArgs{Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["name"] != betaRecord {
			t.Fatalf("Beta user did not receive an isolated scope: %#v", rows)
		}
		companies, err := c.GetList("Test Company", ListArgs{Fields: []string{"name"}})
		if err != nil {
			return err
		}
		if len(companies) != 1 || companies[0]["name"] != "Beta" {
			t.Fatalf("direct entity query ignored Beta scope: %#v", companies)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC01_DocLifecycle(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const alfaUser = "user_alfa@x.com"
	var alfaRecord, betaRecord string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{
			"email": alfaUser, "full_name": alfaUser,
			"roles": []any{map[string]any{"role": "Scope User"}},
		})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, record := range []struct{ title, company string }{
			{"Alfa Record", "Alfa"},
			{"Beta Record", "Beta"},
		} {
			doc, err := c.NewDoc("Test Record", Doc{"title": record.title, "company": record.company})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			if record.company == "Alfa" {
				alfaRecord = saved.Name()
			} else {
				betaRecord = saved.Name()
			}
		}
		permission, err := c.NewDoc("User Permission", Doc{"user": alfaUser, "allow": "Test Company", "for_value": "Alfa"})
		if err != nil {
			return err
		}
		_, err = c.Insert(permission, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, alfaUser, func(c *Ctx) error {
		if _, err := c.GetDoc("Test Record", betaRecord); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("GetDoc Beta Record: expected PermissionError, got %v", err)
		}

		blocked, err := c.NewDoc("Test Record", Doc{"title": "Blocked Insert", "company": "Beta"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(blocked, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("Insert Beta Record: expected PermissionError, got %v", err)
		}

		allowed, err := c.GetDoc("Test Record", alfaRecord)
		if err != nil {
			return err
		}
		allowed["company"] = "Beta"
		if _, err := c.Save(allowed, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("Save Alfa Record as Beta: expected PermissionError, got %v", err)
		}

		if err := c.Delete("Test Record", betaRecord, false, false); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("Delete Beta Record: expected PermissionError, got %v", err)
		}

		return c.WithIgnorePermissions(func() error {
			_, err := c.GetDoc("Test Record", betaRecord)
			return err
		})
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC01_DocLifecycleScopeCannotBeBypassed(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const alfaUser = "user_alfa@x.com"
	var alfaIgnoreSaveRecord, alfaSaveRecord, alfaSubmitRecord, betaRecord string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{
			"email": alfaUser, "full_name": alfaUser,
			"roles": []any{map[string]any{"role": "Scope User"}},
		})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, title := range []string{"Alfa Ignore Save Record", "Alfa Save Record", "Alfa Submit Record", "Beta Record"} {
			company := "Alfa"
			if title == "Beta Record" {
				company = "Beta"
			}
			doc, err := c.NewDoc("Test Record", Doc{"title": title, "company": company})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			switch title {
			case "Alfa Ignore Save Record":
				alfaIgnoreSaveRecord = saved.Name()
			case "Alfa Save Record":
				alfaSaveRecord = saved.Name()
			case "Alfa Submit Record":
				alfaSubmitRecord = saved.Name()
			case "Beta Record":
				betaRecord = saved.Name()
			}
		}
		permission, err := c.NewDoc("User Permission", Doc{"user": alfaUser, "allow": "Test Company", "for_value": "Alfa"})
		if err != nil {
			return err
		}
		_, err = c.Insert(permission, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, alfaUser, func(c *Ctx) error {
		company, err := c.NewDoc("Test Company", Doc{"title": "Gamma"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(company, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Insert scoped entity after naming: expected PermissionError, got %v", err)
		}

		bypassInsert, err := c.NewDoc("Test Record", Doc{"title": "Ignore Permissions Beta", "company": "Beta"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(bypassInsert, SaveOpts{IgnorePermissions: true}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Insert with SaveOpts.IgnorePermissions: expected PermissionError, got %v", err)
		}
		bypassSave, err := c.GetDoc("Test Record", alfaIgnoreSaveRecord)
		if err != nil {
			return err
		}
		bypassSave["company"] = "Beta"
		if _, err := c.Save(bypassSave, SaveOpts{IgnorePermissions: true}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Save with SaveOpts.IgnorePermissions: expected PermissionError, got %v", err)
		}

		if err := c.Delete("Test Record", betaRecord, true, false); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Delete with ignorePerms: expected PermissionError, got %v", err)
		}

		childScope, err := c.NewDoc("Test Record", Doc{
			"title": "Child Beta", "company": "Alfa",
			"items": []any{map[string]any{"company": "Beta"}},
		})
		if err != nil {
			return err
		}
		if _, err := c.Insert(childScope, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Insert child Link outside scope: expected PermissionError, got %v", err)
		}

		hookInsert, err := c.NewDoc("Test Record", Doc{"title": "Hook Insert Beta", "company": "Alfa"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(hookInsert, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Insert after beforeSave hook: expected PermissionError, got %v", err)
		}

		saveDoc, err := c.GetDoc("Test Record", alfaSaveRecord)
		if err != nil {
			return err
		}
		saveDoc["title"] = "Hook Save Beta"
		if _, err := c.Save(saveDoc, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Save after beforeSave hook: expected PermissionError, got %v", err)
		}

		submitDoc, err := c.GetDoc("Test Record", alfaSubmitRecord)
		if err != nil {
			return err
		}
		submitDoc["title"], submitDoc["docstatus"] = "Hook Submit Beta", 1
		if _, err := c.Save(submitDoc, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Errorf("Save after beforeSubmit hook: expected PermissionError, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC01_DirectAccessDynamicLink(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const alfaUser = "user_alfa@x.com"
	var alfaDynamic, betaDynamic, userDynamic string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{
			"email": alfaUser, "full_name": alfaUser,
			"roles": []any{map[string]any{"role": "Scope User"}},
		})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, record := range []struct{ title, partyType, partyName string }{
			{"Alfa Dynamic", "Test Company", "Alfa"},
			{"Beta Dynamic", "Test Company", "Beta"},
			{"User Dynamic", "User", alfaUser},
		} {
			doc, err := c.NewDoc("Test Dynamic Record", Doc{"title": record.title, "party_type": record.partyType, "party_name": record.partyName})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			switch record.title {
			case "Alfa Dynamic":
				alfaDynamic = saved.Name()
			case "Beta Dynamic":
				betaDynamic = saved.Name()
			case "User Dynamic":
				userDynamic = saved.Name()
			}
		}
		permission, err := c.NewDoc("User Permission", Doc{"user": alfaUser, "allow": "Test Company", "for_value": "Alfa"})
		if err != nil {
			return err
		}
		_, err = c.Insert(permission, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, alfaUser, func(c *Ctx) error {
		if _, err := c.GetDoc("Test Dynamic Record", betaDynamic); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("GetDoc Beta Dynamic: expected PermissionError, got %v", err)
		}

		allowed, err := c.GetDoc("Test Dynamic Record", alfaDynamic)
		if err != nil {
			t.Fatalf("GetDoc Alfa Dynamic: expected success, got %v", err)
		}

		if _, err := c.GetDoc("Test Dynamic Record", userDynamic); err != nil {
			t.Fatalf("GetDoc User Dynamic: expected success (selector names an unrestricted DocType), got %v", err)
		}

		allowed["party_name"] = "Beta"
		if _, err := c.Save(allowed, SaveOpts{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("Save Alfa Dynamic moved to Beta: expected PermissionError, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestSEC01_ExistsDBSetAndScopeAdministration covers the by-name exists, the
// direct column write and the administration of User Permission itself.
func TestSEC01_ExistsDBSetAndScopeAdministration(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const (
		scopedManager   = "scoped_manager@x.com"
		unscopedManager = "unscoped_manager@x.com"
		otherUser       = "other_user@x.com"
	)
	var alfaRecord, betaRecord, ownPermission string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, user := range []struct {
			email string
			roles []any
		}{
			{scopedManager, []any{map[string]any{"role": "System Manager"}, map[string]any{"role": "Scope User"}}},
			{unscopedManager, []any{map[string]any{"role": "System Manager"}}},
			{otherUser, []any{map[string]any{"role": "Scope User"}}},
		} {
			u, err := c.NewDoc("User", Doc{"email": user.email, "full_name": user.email, "roles": user.roles})
			if err != nil {
				return err
			}
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		for _, company := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Record", Doc{"title": company + " Record", "company": company})
			if err != nil {
				return err
			}
			saved, err := c.Insert(doc, SaveOpts{})
			if err != nil {
				return err
			}
			if company == "Alfa" {
				alfaRecord = saved.Name()
			} else {
				betaRecord = saved.Name()
			}
		}
		permission, err := c.NewDoc("User Permission", Doc{"user": scopedManager, "allow": "Test Company", "for_value": "Alfa"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(permission, SaveOpts{})
		if err != nil {
			return err
		}
		ownPermission = saved.Name()
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	isPermissionError := func(err error) bool { return err != nil && cerr.From(err).Type == "PermissionError" }

	if err := e.Run(ctx, scopedManager, func(c *Ctx) error {
		if ok, err := c.Exists("Test Record", betaRecord); err != nil || ok {
			t.Errorf("Exists out-of-scope record = %v, %v; want false", ok, err)
		}
		if ok, err := c.Exists("Test Record", alfaRecord); err != nil || !ok {
			t.Errorf("Exists in-scope record = %v, %v; want true", ok, err)
		}

		if _, err := c.DBSet("Test Record", betaRecord, Doc{"title": "Beta Changed"}, true); !isPermissionError(err) {
			t.Errorf("DBSet out-of-scope record: expected PermissionError, got %v", err)
		}
		if _, err := c.DBSet("Test Record", alfaRecord, Doc{"company": "Beta"}, true); !isPermissionError(err) {
			t.Errorf("DBSet moving record out of scope: expected PermissionError, got %v", err)
		}
		if _, err := c.DBSet("Test Record", alfaRecord, Doc{"title": "Alfa Changed"}, true); err != nil {
			t.Errorf("DBSet inside scope: expected success, got %v", err)
		}

		own, err := c.GetDocIgnoringPerms("User Permission", ownPermission)
		if err != nil {
			return err
		}
		for _, opts := range []SaveOpts{{}, {IgnorePermissions: true}} {
			grant, err := c.NewDoc("User Permission", Doc{"user": scopedManager, "allow": "Test Company", "for_value": "Beta"})
			if err != nil {
				return err
			}
			if _, err := c.Insert(grant, opts); !isPermissionError(err) {
				t.Errorf("Insert User Permission (ignorePermissions=%v): expected PermissionError, got %v", opts.IgnorePermissions, err)
			}
			changed := Doc{}
			for k, v := range own {
				changed[k] = v
			}
			changed["for_value"] = "Beta"
			if _, err := c.Save(changed, opts); !isPermissionError(err) {
				t.Errorf("Save User Permission (ignorePermissions=%v): expected PermissionError, got %v", opts.IgnorePermissions, err)
			}
		}
		if _, err := c.DBSet("User Permission", ownPermission, Doc{"for_value": "Beta"}, true); !isPermissionError(err) {
			t.Errorf("DBSet User Permission: expected PermissionError, got %v", err)
		}
		for _, ignore := range []bool{false, true} {
			if err := c.Delete("User Permission", ownPermission, ignore, false); !isPermissionError(err) {
				t.Errorf("Delete User Permission (ignorePerms=%v): expected PermissionError, got %v", ignore, err)
			}
		}
		if rows, err := c.GetList("User Permission", ListArgs{IgnorePermissions: true}); err != nil || len(rows) != 0 {
			t.Errorf("GetList User Permission ignoring permissions = %v, %v; want no rows", rows, err)
		}
		if ok, err := c.Exists("User Permission", ownPermission); err != nil || ok {
			t.Errorf("Exists User Permission = %v, %v; want false", ok, err)
		}
		if ok, err := c.HasPermission("User Permission", "read", nil); err != nil || ok {
			t.Errorf("HasPermission read User Permission = %v, %v; want false", ok, err)
		}
		// the user's own scope is still read by the engine
		if perms, err := c.UserPermissions(); err != nil || len(perms) != 1 || perms[0].ForValue != "Alfa" {
			t.Errorf("UserPermissions = %v, %v; want the Alfa rule", perms, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	scopeCount := func(user string) int {
		t.Helper()
		var n int
		if err := e.Run(ctx, user, func(c *Ctx) error {
			perms, err := c.UserPermissions()
			n = len(perms)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return n
	}
	grants := func() int64 {
		t.Helper()
		n, err := e.CountAuditEvents(ctx, AuditFilter{Action: "permission.scope_grant", TargetDocType: "User", TargetName: otherUser})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := scopeCount(otherUser); n != 0 {
		t.Fatalf("other user scope rows before grant = %d, want 0", n)
	}
	grantsBefore := grants()
	var granted string
	if err := e.Run(ctx, unscopedManager, func(c *Ctx) error {
		doc, err := c.NewDoc("User Permission", Doc{"user": otherUser, "allow": "Test Company", "for_value": "Alfa"})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		granted = saved.Name()
		saved["for_value"] = "Beta"
		_, err = c.Save(saved, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("unscoped System Manager insert/save User Permission: %v", err)
	}
	if n := scopeCount(otherUser); n != 1 {
		t.Errorf("other user scope rows after grant = %d, want 1", n)
	}
	if got := grants(); got != grantsBefore+2 {
		t.Errorf("scope_grant audit count = %d, want %d", got, grantsBefore+2)
	}
	if err := e.Run(ctx, unscopedManager, func(c *Ctx) error {
		return c.Delete("User Permission", granted, false, false)
	}); err != nil {
		t.Fatalf("unscoped System Manager delete User Permission: %v", err)
	}
	if n := scopeCount(otherUser); n != 0 {
		t.Errorf("other user scope rows after delete = %d, want 0", n)
	}
}

// TestSEC01_DBSetChildRowScopeUsesParentApplicableFor: a scope rule with
// applicable_for "Test Record" restricts a Link field on a Test Record Item
// child row the same way Save checks it — under the parent DocType, not the
// child's own name. A direct DBSet on the child row must resolve the same
// applicable_for, or a scope limited to one DocType would silently stop
// applying to that DocType's own child rows.
func TestSEC01_DBSetChildRowScopeUsesParentApplicableFor(t *testing.T) {
	e := setupSEC01(t)
	ctx := context.Background()
	const scopedUser = "child_scope_user@x.com"
	var itemName string

	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		u, err := c.NewDoc("User", Doc{"email": scopedUser, "full_name": scopedUser,
			"roles": []any{map[string]any{"role": "Scope User"}}})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		for _, name := range []string{"Alfa", "Beta"} {
			doc, err := c.NewDoc("Test Company", Doc{"title": name})
			if err != nil {
				return err
			}
			if _, err := c.Insert(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		record, err := c.NewDoc("Test Record", Doc{
			"title": "With Item", "company": "Alfa",
			"items": []any{map[string]any{"company": "Alfa"}},
		})
		if err != nil {
			return err
		}
		saved, err := c.Insert(record, SaveOpts{})
		if err != nil {
			return err
		}
		full, err := c.GetDoc("Test Record", saved.Name())
		if err != nil {
			return err
		}
		items := full.Children("items")
		if len(items) != 1 {
			t.Fatalf("expected one item row, got %#v", full["items"])
		}
		itemName = items[0].Name()

		// Scoped to "Alfa" only for "Test Record": without resolving the
		// child row's parenttype, a direct DBSet on "Test Record Item" would
		// check applicable_for "Test Record Item", not match this rule, and
		// skip the restriction entirely.
		permission, err := c.NewDoc("User Permission", Doc{
			"user": scopedUser, "allow": "Test Company", "for_value": "Alfa", "applicable_for": "Test Record",
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(permission, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, scopedUser, func(c *Ctx) error {
		if _, err := c.DBSet("Test Record Item", itemName, Doc{"company": "Beta"}, true); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("DBSet moving a child row out of the parent's scope: expected PermissionError, got %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func hasName(rows []map[string]any, want string) bool {
	for _, row := range rows {
		if row["name"] == want {
			return true
		}
	}
	return false
}
