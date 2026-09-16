package engine

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/js"
)

const (
	sec02Staff   = "staff@x.com"
	sec02HR      = "hr@x.com"
	sec02Auditor = "auditor@x.com"
)

func sec02App(t *testing.T) string {
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
export default defineApp({ name: "fieldperm_test", title: "Field Permission Test", roles: ["Staff", "HR", "Auditor"] });`)
	write("doctypes/employee/employee.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee", naming: { field: "title" }, submittable: true, trackChanges: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "department", fieldtype: "Data", label: "Department" },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary", permlevel: 1 },
    { fieldname: "review", fieldtype: "Small Text", label: "Review", permlevel: 2 },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Employee Line" },
    { fieldname: "bonuses", fieldtype: "Table", label: "Bonuses", options: "Employee Bonus", permlevel: 1 },
    { fieldname: "amended_from", fieldtype: "Link", label: "Amended From", options: "Employee", readOnly: true },
  ],
  permissions: [
    { role: "Staff", read: true, write: true, create: true, submit: true, cancel: true, amend: true },
    { role: "HR", read: true, write: true, create: true, submit: true, cancel: true, amend: true },
    { role: "HR", permlevel: 1, read: true, write: true },
    { role: "HR", permlevel: 2, read: true },
    { role: "Auditor", permlevel: 1, read: true },
  ] });`)
	write("doctypes/employee/employee.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Employee", {
  beforeSave(doc) { if (doc.department === "Hooked") doc.salary = 42; },
});`)
	write("doctypes/employee_line/employee_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee Line", isChild: true, fields: [
  { fieldname: "label", fieldtype: "Data", label: "Label" },
  { fieldname: "amount", fieldtype: "Currency", label: "Amount", permlevel: 1 },
] });`)
	write("doctypes/employee_bonus/employee_bonus.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee Bonus", isChild: true, fields: [
  { fieldname: "value", fieldtype: "Currency", label: "Value" },
] });`)
	return dir
}

func setupSEC02(t *testing.T) *Engine {
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
	e0.DB.Pool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName)
	if _, err := e0.DB.Pool.Exec(ctx, "CREATE DATABASE "+dbName); err != nil {
		e0.DB.Close()
		t.Fatal(err)
	}
	e0.DB.Close()
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "fieldperm_test", Dir: sec02App(t)}}, Test: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Migrate(ctx, false); err != nil {
		e.DB.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() { e.DB.Close() })
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		for user, role := range map[string]string{sec02Staff: "Staff", sec02HR: "HR", sec02Auditor: "Auditor"} {
			u, _ := c.NewDoc("User", Doc{"email": user, "full_name": user, "roles": []any{map[string]any{"role": role}}})
			if _, err := c.Insert(u, SaveOpts{}); err != nil {
				return err
			}
		}
		doc, _ := c.NewDoc("Employee", Doc{
			"title": "Ana", "department": "Ops", "salary": 100, "review": "good",
			"lines":   []any{map[string]any{"label": "base", "amount": 5}},
			"bonuses": []any{map[string]any{"value": 7}},
		})
		_, err := c.Insert(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return e
}

func wantPermissionError(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || cerr.From(err).Type != "PermissionError" {
		t.Fatalf("%s: want PermissionError, got %v", what, err)
	}
}

func TestSEC02_ReadRedaction(t *testing.T) {
	e := setupSEC02(t)
	ctx := context.Background()

	read := func(user string) Doc {
		var out Doc
		if err := e.Run(ctx, user, func(c *Ctx) error {
			doc, err := c.GetDoc("Employee", "Ana")
			if err != nil {
				return err
			}
			out = c.RedactDoc("Employee", doc)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return out
	}

	staff := read(sec02Staff)
	for _, k := range []string{"salary", "review", "bonuses"} {
		if _, ok := staff[k]; ok {
			t.Fatalf("staff sees %s: %v", k, staff)
		}
	}
	if staff.Str("department") != "Ops" {
		t.Fatalf("level-0 field missing: %v", staff)
	}
	lines := staff.Children("lines")
	if len(lines) != 1 || lines[0].Str("label") != "base" {
		t.Fatalf("lines = %v", lines)
	}
	if _, ok := lines[0]["amount"]; ok {
		t.Fatalf("staff sees child amount: %v", lines[0])
	}

	hr := read(sec02HR)
	if toFloat(hr["salary"]) != 100 || hr.Str("review") != "good" || len(hr.Children("bonuses")) != 1 {
		t.Fatalf("HR must see levels 1 and 2: %v", hr)
	}
	if toFloat(hr.Children("lines")[0]["amount"]) != 5 {
		t.Fatalf("HR must see child amount: %v", hr.Children("lines"))
	}

	admin := read("Administrator")
	if toFloat(admin["salary"]) != 100 {
		t.Fatalf("Administrator must see everything: %v", admin)
	}
}

func TestSEC02_LevelRowNeverGrantsTheDocument(t *testing.T) {
	e := setupSEC02(t)
	if err := e.Run(context.Background(), sec02Auditor, func(c *Ctx) error {
		if ok, _ := c.HasPermission("Employee", "read", nil); ok {
			t.Fatal("a permlevel 1 row granted document read")
		}
		_, err := c.GetDoc("Employee", "Ana")
		wantPermissionError(t, "auditor get", err)
		_, err = c.GetList("Employee", ListArgs{Fields: []string{"name"}})
		wantPermissionError(t, "auditor list", err)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC02_ListQueries(t *testing.T) {
	e := setupSEC02(t)
	ctx := context.Background()
	if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
		rows, err := c.GetList("Employee", ListArgs{Fields: []string{"*"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["department"] != "Ops" {
			t.Fatalf("rows = %v", rows)
		}
		if _, ok := rows[0]["salary"]; ok {
			t.Fatalf("* exposed salary: %v", rows[0])
		}
		rows, err = c.GetList("Employee", ListArgs{Fields: []string{"name", "salary", "salary as pay", "Employee Line.amount"}})
		if err != nil {
			return err
		}
		if len(rows[0]) != 1 {
			t.Fatalf("restricted columns must be omitted, got %v", rows[0])
		}
		rows, err = c.GetList("Employee", ListArgs{Fields: []string{"salary"}})
		if err != nil || len(rows) != 1 || rows[0]["name"] != "Ana" {
			t.Fatalf("a list of only restricted fields falls back to name: %v %v", rows, err)
		}
		for what, args := range map[string]ListArgs{
			"filter":       {Filters: []any{[]any{"salary", ">", 50}}},
			"or filter":    {OrFilters: []any{[]any{"salary", ">", 50}}},
			"child filter": {Filters: []any{[]any{"Employee Line.amount", ">", 1}}},
			"table filter": {Filters: []any{[]any{"Employee Bonus.value", ">", 1}}},
			"order":        {OrderBy: "salary desc"},
			"group":        {Fields: []string{"count(*)"}, GroupBy: "salary"},
			"aggregate":    {Fields: []string{"sum(salary)"}},
		} {
			_, err := c.GetList("Employee", args)
			wantPermissionError(t, what, err)
		}
		// privileged queries are unaffected
		rows, err = c.GetList("Employee", ListArgs{Fields: []string{"salary"}, Filters: []any{[]any{"salary", ">", 50}}, IgnorePermissions: true})
		if err != nil || len(rows) != 1 || toFloat(rows[0]["salary"]) != 100 {
			t.Fatalf("ignorePermissions list: %v %v", rows, err)
		}
		// a child listing is as restricted as its parent
		rows, err = c.GetList("Employee Line", ListArgs{Fields: []string{"*"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 {
			t.Fatalf("child rows = %v", rows)
		}
		if _, ok := rows[0]["amount"]; ok {
			t.Fatalf("child * exposed amount: %v", rows[0])
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, sec02HR, func(c *Ctx) error {
		rows, err := c.GetList("Employee", ListArgs{Fields: []string{"name", "salary"}, Filters: []any{[]any{"salary", ">", 50}}, OrderBy: "salary desc"})
		if err != nil || len(rows) != 1 || toFloat(rows[0]["salary"]) != 100 {
			t.Fatalf("HR list: %v %v", rows, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC02_Writes(t *testing.T) {
	e := setupSEC02(t)
	ctx := context.Background()

	// the desk round trip: read redacted, change a visible field, save
	stored := func() Doc {
		var out Doc
		e.Run(ctx, "Administrator", func(c *Ctx) error {
			out, _ = c.GetDoc("Employee", "Ana")
			return nil
		})
		return out
	}
	if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
		doc, err := c.GetDoc("Employee", "Ana")
		if err != nil {
			return err
		}
		doc = c.RedactDoc("Employee", doc)
		doc["department"] = "Sales"
		doc.Children("lines")[0]["label"] = "renamed"
		doc["salary"] = nil // an API client echoing a blank value
		_, err = c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	s := stored()
	if s.Str("department") != "Sales" || toFloat(s["salary"]) != 100 || s.Str("review") != "good" {
		t.Fatalf("hidden values were not preserved: %v", s)
	}
	if len(s.Children("bonuses")) != 1 || s.Children("lines")[0].Str("label") != "renamed" || toFloat(s.Children("lines")[0]["amount"]) != 5 {
		t.Fatalf("hidden child values were not preserved: %v", s)
	}

	attempt := func(user, what string, mutate func(Doc)) error {
		var saveErr error
		if err := e.Run(ctx, user, func(c *Ctx) error {
			doc, err := c.GetDoc("Employee", "Ana")
			if err != nil {
				return err
			}
			doc = c.RedactDoc("Employee", doc)
			mutate(doc)
			_, saveErr = c.Save(doc, SaveOpts{})
			return nil
		}); err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		return saveErr
	}
	wantPermissionError(t, "staff sets salary", attempt(sec02Staff, "salary", func(d Doc) { d["salary"] = 1 }))
	wantPermissionError(t, "staff adds bonus", attempt(sec02Staff, "bonus", func(d Doc) {
		d["bonuses"] = []any{map[string]any{"value": 1}}
	}))
	wantPermissionError(t, "staff sets child amount", attempt(sec02Staff, "amount", func(d Doc) {
		d.Children("lines")[0]["amount"] = 9
	}))
	wantPermissionError(t, "staff adds row with amount", attempt(sec02Staff, "row", func(d Doc) {
		d["lines"] = append(d["lines"].([]any), map[string]any{"label": "x", "amount": 3})
	}))
	if err := attempt(sec02Staff, "row without amount", func(d Doc) {
		d["lines"] = append(d["lines"].([]any), map[string]any{"label": "extra"})
	}); err != nil {
		t.Fatalf("a new row with only visible values must save: %v", err)
	}
	wantPermissionError(t, "HR changes read-only level 2", attempt(sec02HR, "review", func(d Doc) { d["review"] = "bad" }))
	if err := attempt(sec02HR, "HR salary", func(d Doc) {
		d["salary"] = 200
		d["bonuses"] = []any{}
	}); err != nil {
		t.Fatalf("HR writes level 1: %v", err)
	}
	if s := stored(); toFloat(s["salary"]) != 200 || len(s.Children("bonuses")) != 0 {
		t.Fatalf("HR write lost: %v", s)
	}
	// a controller may set a restricted field on behalf of the user
	if err := attempt(sec02Staff, "hook", func(d Doc) { d["department"] = "Hooked" }); err != nil {
		t.Fatalf("hook write: %v", err)
	}
	if s := stored(); toFloat(s["salary"]) != 42 {
		t.Fatalf("hook did not set salary: %v", s)
	}

	// inserts
	if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
		doc, _ := c.NewDoc("Employee", Doc{"title": "Bia", "salary": 10})
		_, err := c.Insert(doc, SaveOpts{})
		wantPermissionError(t, "staff inserts salary", err)
		doc, _ = c.NewDoc("Employee", Doc{"title": "Bia", "lines": []any{map[string]any{"label": "a"}}})
		if _, err := c.Insert(doc, SaveOpts{}); err != nil {
			t.Fatalf("staff insert without restricted values: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSEC02_AmendCarriesRestrictedValues(t *testing.T) {
	e := setupSEC02(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, _ := c.GetDoc("Employee", "Ana")
		doc["docstatus"] = 1
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			return err
		}
		doc, _ = c.GetDoc("Employee", "Ana")
		doc["docstatus"] = 2
		_, err := c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var amended string
	if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
		doc, err := c.Amend("Employee", "Ana")
		if err != nil {
			return err
		}
		doc = c.RedactDoc("Employee", doc)
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		amended = saved.Name()
		return nil
	}); err != nil {
		t.Fatalf("staff amend: %v", err)
	}
	e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Employee", amended)
		if err != nil {
			t.Fatal(err)
		}
		if toFloat(doc["salary"]) != 100 || len(doc.Children("bonuses")) != 1 || toFloat(doc.Children("lines")[0]["amount"]) != 5 {
			t.Fatalf("amendment lost restricted values: %v", doc)
		}
		return nil
	})
}
