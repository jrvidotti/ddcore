package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

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
	write("doctypes/test_record/test_record.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Test Record", naming: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "company", fieldtype: "Link", label: "Company", options: "Test Company" },
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
	const user = "user_alfa@x.com"
	var alfaRecord string

	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
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
			{"Unassigned Record", ""},
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
			}
		}
		permission, err := c.NewDoc("User Permission", Doc{
			"user": user, "allow": "Test Company", "for_value": "Alfa",
		})
		if err != nil {
			return err
		}
		_, err = c.Insert(permission, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	if err := e.Run(ctx, user, func(c *Ctx) error {
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
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
