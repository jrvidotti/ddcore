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
  permissions: [{ role: "Scope User", read: true }] });`)
	write("doctypes/test_division/test_division.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Division", naming: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Scope User", read: true }] });`)
	write("doctypes/test_record/test_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record", naming: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Test Company" },
    { fieldname: "division", fieldtype: "Link", label: "Division", options: "Test Division" },
  ],
  permissions: [{ role: "Scope User", read: true, create: true, write: true, delete: true }] });`)
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

	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
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

	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
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

func hasName(rows []map[string]any, want string) bool {
	for _, row := range rows {
		if row["name"] == want {
			return true
		}
	}
	return false
}
