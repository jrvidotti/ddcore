package engine

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

const (
	vSales   = "sales@x.com"   // reads Party and Person, not Organization or salaries
	vManager = "manager@x.com" // reads everything, salaries included
)

// virtualApp is issue #17's shape: Party = Person ∪ Organization, and a
// Supplier whose Link accepts either.
func virtualApp(t *testing.T) string {
	dir := t.TempDir()
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644)
	}
	w("ddcore.app.ts", `import { defineApp } from "@ddcore/sdk";
export default defineApp({ name: "demo", title: "Demo", roles: ["Sales", "Manager"] });`)
	w("doctypes/city/city.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "City", idGeneration: { field: "city_name" }, titleField: "city_name",
  fields: [{ fieldname: "city_name", fieldtype: "Data", label: "Name", reqd: true }],
  permissions: [{ role: "Sales", read: true }, { role: "Manager", read: true, write: true, create: true }] });`)
	w("doctypes/person/person.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Person", idGeneration: { field: "cpf" }, allowRename: true, titleField: "person_name",
  fields: [
    { fieldname: "person_name", fieldtype: "Data", label: "Name", reqd: true },
    { fieldname: "cpf", fieldtype: "Data", label: "CPF", reqd: true },
    { fieldname: "city", fieldtype: "Link", label: "City", options: "City" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary", permlevel: 1 },
  ],
  permissions: [
    { role: "Sales", read: true },
    { role: "Manager", read: true, write: true, create: true, delete: true },
    { role: "Manager", permlevel: 1, read: true, write: true },
  ] });`)
	w("doctypes/person/person.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Person", {
  permissionQuery(user) { return user === "`+vSales+`" ? { person_name: ["!=", "Hidden"] } : []; },
});`)
	w("doctypes/organization/organization.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Organization", idGeneration: { field: "cnpj" }, titleField: "organization_name",
  fields: [
    { fieldname: "organization_name", fieldtype: "Data", label: "Name", reqd: true },
    { fieldname: "cnpj", fieldtype: "Data", label: "CNPJ", reqd: true },
    { fieldname: "city", fieldtype: "Link", label: "City", options: "City" },
  ],
  permissions: [{ role: "Manager", read: true, write: true, create: true, delete: true, share: true }] });`)
	w("doctypes/party/party.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Party", titleField: "party_name", searchFields: ["party_name", "tax_id"],
  virtual: { sources: [
    { doctype: "Person", fields: { party_name: "person_name", tax_id: "cpf", city: "city", pay: "salary" } },
    { doctype: "Organization", fields: { party_name: "organization_name", tax_id: "cnpj", city: "city" } },
  ] },
  fields: [
    { fieldname: "party_name", fieldtype: "Data", label: "Name", inListView: true },
    { fieldname: "tax_id", fieldtype: "Data", label: "Tax ID", inListView: true },
    { fieldname: "city", fieldtype: "Link", label: "City", options: "City" },
    { fieldname: "pay", fieldtype: "Currency", label: "Pay" },
  ],
  permissions: [{ role: "Sales", read: true }, { role: "Manager", read: true, report: true, export: true }] });`)
	w("doctypes/supplier/supplier.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Supplier",
  fields: [
    { fieldname: "party", fieldtype: "Link", label: "Party", options: "Party", reqd: true },
    { fieldname: "party_name", fieldtype: "Data", label: "Party name", fetchFrom: "party.party_name", readOnly: true },
  ],
  permissions: [{ role: "Manager", read: true, write: true, create: true, delete: true }] });`)
	return dir
}

func setupVirtual(t *testing.T) *Engine {
	ctx := context.Background()
	adminDSN, dbName := adminDSNFor(testDSN)
	e0, err := New(ctx, Config{DSN: adminDSN})
	if err != nil {
		if os.Getenv("DDCORE_TEST_DSN") != "" {
			t.Fatalf("postgres unavailable at DDCORE_TEST_DSN: %v", err)
		}
		t.Skipf("postgres unavailable: %v", err)
	}
	e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		t.Fatal(err)
	}
	e0.DB.Close()
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "demo", Dir: virtualApp(t)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		t.Fatal(err)
	}
	if plan, _ := e.Plan(ctx, false); len(plan) != 0 {
		t.Fatalf("migrate is not idempotent: %v", plan)
	}
	t.Cleanup(func() { e.DB.Close() })
	runAs(t, e, "Admin", func(c *Ctx) error {
		for _, u := range []struct{ email, role string }{{vSales, "Sales"}, {vManager, "Manager"}} {
			d, _ := c.NewDoc("User", Doc{"email": u.email, "full_name": u.email})
			d["roles"] = []any{map[string]any{"role": u.role}}
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		ins := func(dt string, d Doc) error {
			doc, _ := c.NewDoc(dt, d)
			_, err := c.Insert(doc, SaveOpts{})
			return err
		}
		for _, city := range []string{"Curitiba", "Recife"} {
			if err := ins("City", Doc{"city_name": city}); err != nil {
				return err
			}
		}
		for _, p := range []Doc{
			{"person_name": "Ana", "cpf": "111", "city": "Curitiba", "salary": 5000},
			{"person_name": "Bruno", "cpf": "222", "city": "Recife", "salary": 7000},
			{"person_name": "Hidden", "cpf": "333"},
		} {
			if err := ins("Person", p); err != nil {
				return err
			}
		}
		for _, o := range []Doc{
			{"organization_name": "Acme", "cnpj": "999", "city": "Curitiba"},
			// the same id as a Person: the union keeps them apart by prefix
			{"organization_name": "Twin", "cnpj": "111", "city": "Recife"},
		} {
			if err := ins("Organization", o); err != nil {
				return err
			}
		}
		return nil
	})
	return e
}

func vids(rows []map[string]any) []string {
	var out []string
	for _, r := range rows {
		out = append(out, r["id"].(string))
	}
	sort.Strings(out)
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestVirtualListUnion(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, vManager, func(c *Ctx) error {
		rows, err := c.GetList("Party", ListArgs{Fields: []string{"*"}})
		if err != nil {
			return err
		}
		want := []string{"Organization:111", "Organization:999", "Person:111", "Person:222", "Person:333"}
		if got := vids(rows); !eq(got, want) {
			t.Fatalf("union = %v, want %v", got, want)
		}
		for _, r := range rows {
			if r["id"] == "Person:222" && (r["party_name"] != "Bruno" || r["tax_id"] != "222" || r["source_doctype"] != "Person" || r["city"] != "Recife") {
				t.Fatalf("mapped row = %v", r)
			}
			if r["id"] == "Organization:999" && r["pay"] != nil {
				t.Fatalf("an unmapped field reads as null: %v", r)
			}
		}
		// filters, order and paging run over the union
		rows, err = c.GetList("Party", ListArgs{Filters: map[string]any{"city": "Curitiba"}, Fields: []string{"id", "party_name"}, OrderBy: "party_name asc"})
		if err != nil {
			return err
		}
		if len(rows) != 2 || rows[0]["party_name"] != "Acme" || rows[1]["party_name"] != "Ana" {
			t.Fatalf("filtered and ordered = %v", rows)
		}
		rows, err = c.GetList("Party", ListArgs{Fields: []string{"id"}, OrderBy: "id asc", Limit: 2, Start: 1})
		if err != nil {
			return err
		}
		if got := vids(rows); !eq(got, []string{"Organization:999", "Person:111"}) {
			t.Fatalf("paged = %v", got)
		}
		rows, err = c.GetList("Party", ListArgs{Filters: map[string]any{"source_doctype": "Organization"}})
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("source filter = %v", rows)
		}
		n, err := c.Count("Party", map[string]any{"tax_id": "111"})
		if err != nil {
			return err
		}
		if n != 2 {
			t.Fatalf("count = %d", n)
		}
		// a `like` on a Link column still matches the target's title
		rows, err = c.GetList("Party", ListArgs{Filters: []any{[]any{"city", "like", "%recif%"}}})
		if err != nil {
			return err
		}
		if len(rows) != 2 {
			t.Fatalf("like on Link = %v", rows)
		}
		hits, err := c.LinkSearch("Party", "acm", nil, 10)
		if err != nil {
			return err
		}
		if len(hits) != 1 || hits[0]["id"] != "Organization:999" {
			t.Fatalf("link search = %v", hits)
		}
		grouped, err := c.GetList("Party", ListArgs{Fields: []string{"source_doctype", "count(*) as n"}, GroupBy: "source_doctype"})
		if err != nil {
			return err
		}
		if len(grouped) != 2 {
			t.Fatalf("group by = %v", grouped)
		}
		return nil
	})
}

func TestVirtualPermissionsFollowSources(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, vSales, func(c *Ctx) error {
		rows, err := c.GetList("Party", ListArgs{Fields: []string{"id", "pay"}})
		if err != nil {
			return err
		}
		// no Organization (unreadable), no Hidden (permissionQuery)
		if got := vids(rows); !eq(got, []string{"Person:111", "Person:222"}) {
			t.Fatalf("sales sees %v", got)
		}
		for _, r := range rows {
			if r["pay"] != nil {
				t.Fatalf("salary sits above sales' level on Person, so pay is null: %v", r)
			}
		}
		// filtering on a masked field finds nothing rather than leaking it
		if n, err := c.Count("Party", []any{[]any{"pay", ">", 0}}); err != nil || n != 0 {
			t.Fatalf("a filter on a masked field matched %d rows", n)
		}
		if _, err := c.GetDoc("Party", "Organization:999"); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("an unreadable source's row: want PermissionError, got %v", err)
		}
		if _, err := c.GetDoc("Party", "Person:nope"); err == nil || cerr.From(err).Type != "DoesNotExistError" {
			t.Fatalf("a missing row: want DoesNotExistError, got %v", err)
		}
		if _, err := c.GetDoc("Party", "City:Recife"); err == nil || cerr.From(err).Type != "DoesNotExistError" {
			t.Fatalf("a DocType that is not a source: want DoesNotExistError, got %v", err)
		}
		doc, err := c.GetDoc("Party", "Person:111")
		if err != nil {
			return err
		}
		if doc["party_name"] != "Ana" || doc["doctype"] != "Party" {
			t.Fatalf("get = %v", doc)
		}
		return nil
	})
	runAs(t, e, vManager, func(c *Ctx) error {
		rows, err := c.GetList("Party", ListArgs{Filters: map[string]any{"id": "Person:222"}, Fields: []string{"pay"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || toFloat(rows[0]["pay"]) != 7000 {
			t.Fatalf("manager reads the salary: %v", rows)
		}
		_, err = c.ShareDoc("Organization", "999", vSales, ShareRights{})
		return err
	})
	runAs(t, e, vSales, func(c *Ctx) error {
		rows, err := c.GetList("Party", ListArgs{Filters: map[string]any{"source_doctype": "Organization"}})
		if err != nil {
			return err
		}
		if got := vids(rows); !eq(got, []string{"Organization:999"}) {
			t.Fatalf("a share grants its one row: %v", got)
		}
		return nil
	})
	// a user without a role on Party cannot list it, whatever the sources say
	runAs(t, e, "Admin", func(c *Ctx) error {
		u, _ := c.NewDoc("User", Doc{"email": "nobody@x.com", "full_name": "Nobody"})
		_, err := c.Insert(u, SaveOpts{})
		return err
	})
	runAs(t, e, "nobody@x.com", func(c *Ctx) error {
		if _, err := c.GetList("Party", ListArgs{}); err == nil || cerr.From(err).Type != "PermissionError" {
			t.Fatalf("want PermissionError, got %v", err)
		}
		return nil
	})
}

func TestVirtualScopesFollowSources(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		up, _ := c.NewDoc("User Permission", Doc{"user": vManager, "allow": "City", "for_value": "Curitiba"})
		if _, err := c.Insert(up, SaveOpts{}); err != nil {
			return err
		}
		bad, _ := c.NewDoc("User Permission", Doc{"user": vManager, "allow": "Party", "for_value": "Person:111"})
		if _, err := c.Insert(bad, SaveOpts{}); err == nil {
			t.Fatal("a scope over a virtual DocType must be refused")
		}
		return nil
	})
	runAs(t, e, vManager, func(c *Ctx) error {
		rows, err := c.GetList("Party", ListArgs{})
		if err != nil {
			return err
		}
		// a Link-to-City scope applies inside each source; Hidden, with no city, is outside it as it is on Person
		if got := vids(rows); !eq(got, []string{"Organization:999", "Person:111"}) {
			t.Fatalf("scoped = %v", got)
		}
		return nil
	})
}

func TestVirtualLinks(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, vManager, func(c *Ctx) error {
		s, _ := c.NewDoc("Supplier", Doc{"party": "Organization:999"})
		s, err := c.Insert(s, SaveOpts{})
		if err != nil {
			return err
		}
		if s["party_name"] != "Acme" {
			t.Fatalf("fetchFrom through a virtual Link = %v", s["party_name"])
		}
		for _, bad := range []string{"Organization:nope", "City:Recife", "999", "Nobody:1"} {
			d, _ := c.NewDoc("Supplier", Doc{"party": bad})
			if _, err := c.Insert(d, SaveOpts{}); err == nil {
				t.Fatalf("Link value %q accepted", bad)
			}
		}
		titles, err := c.LinkTitles("Party", []string{"Organization:999", "Person:111", "Person:none"})
		if err != nil {
			return err
		}
		if titles["Organization:999"] != "Acme" || titles["Person:111"] != "Ana" || titles["Person:none"] != "Person:none" {
			t.Fatalf("titles = %v", titles)
		}
		got := c.ResolveLinkTitles("Supplier", s)
		if got["Party"]["Organization:999"] != "Acme" {
			t.Fatalf("resolved titles = %v", got)
		}
		// a referenced source cannot be deleted
		if err := c.Delete("Organization", "999", false, false); err == nil || cerr.From(err).Type != "LinkExistsError" {
			t.Fatalf("delete a linked source: want LinkExistsError, got %v", err)
		}
		// a renamed source carries its Links along
		p, _ := c.NewDoc("Supplier", Doc{"party": "Person:222"})
		p, err = c.Insert(p, SaveOpts{})
		if err != nil {
			return err
		}
		if _, err := c.Rename("Person", "222", "444"); err != nil {
			return err
		}
		v, _ := c.GetValue("Supplier", p.ID(), "party")
		if v != "Person:444" {
			t.Fatalf("after rename the Link reads %v", v)
		}
		return nil
	})
}

func TestVirtualWritesRefused(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, "Admin", func(c *Ctx) error {
		wantRefused := func(what string, err error) {
			t.Helper()
			if err == nil || cerr.From(err).Type != "ValidationError" {
				t.Fatalf("%s: want ValidationError, got %v", what, err)
			}
		}
		d, err := c.NewDoc("Party", Doc{"party_name": "X"})
		if err == nil {
			_, err = c.Insert(d, SaveOpts{})
		}
		wantRefused("insert", err)
		doc, err := c.GetDoc("Party", "Person:111")
		if err != nil {
			return err
		}
		_, err = c.Save(doc, SaveOpts{})
		wantRefused("save", err)
		_, err = c.DBSet("Party", "Person:111", Doc{"party_name": "Y"}, true)
		wantRefused("dbSet", err)
		wantRefused("delete", c.Delete("Party", "Person:111", false, true))
		_, err = c.Rename("Party", "Person:111", "Person:555")
		wantRefused("rename", err)
		_, err = c.ImportDoc(Doc{"doctype": "Party", "id": "Person:9", "party_name": "Z"}, ImportOpts{})
		wantRefused("import", err)
		_, err = c.CanDataImport("Party", "insert")
		wantRefused("data import", err)
		return nil
	})
}

func TestVirtualExport(t *testing.T) {
	e := setupVirtual(t)
	runAs(t, e, vManager, func(c *Ctx) error {
		col := &collector{}
		if _, err := c.Export(ExportArgs{Doctype: "Party", Batch: 2}, col); err != nil {
			return err
		}
		if len(col.docs) != 5 {
			t.Fatalf("export paged through %d rows: %v", len(col.docs), col.names())
		}
		return nil
	})
}
