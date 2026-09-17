package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/db"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/print"
)

const (
	sec02Staff   = "staff@x.com"
	sec02HR      = "hr@x.com"
	sec02Auditor = "auditor@x.com"
)

func sec02App(t *testing.T, extra map[string]string) string {
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
    { fieldname: "cost_center", fieldtype: "Link", label: "Cost Center", options: "Cost Center", permlevel: 1 },
    { fieldname: "salary", fieldtype: "Currency", label: "Salary", permlevel: 1 },
    { fieldname: "review", fieldtype: "Small Text", label: "Review", permlevel: 2 },
    { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Employee Line" },
    { fieldname: "bonuses", fieldtype: "Table", label: "Bonuses", options: "Employee Bonus", permlevel: 1 },
    { fieldname: "amended_from", fieldtype: "Link", label: "Amended From", options: "Employee", readOnly: true },
    { fieldname: "contract", fieldtype: "Attach", label: "Contract", permlevel: 1 },
  ],
  permissions: [
    { role: "Staff", read: true, write: true, create: true, submit: true, cancel: true, amend: true, export: true },
    { role: "HR", read: true, write: true, create: true, submit: true, cancel: true, amend: true, export: true },
    { role: "HR", permlevel: 1, read: true, write: true },
    { role: "HR", permlevel: 2, read: true },
    { role: "Auditor", permlevel: 1, read: true },
  ] });`)
	write("doctypes/employee/employee.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Employee", {
  beforeSave(doc) { if (doc.department === "Hooked") doc.salary = 42; },
});`)
	write("doctypes/cost_center/cost_center.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Cost Center", naming: { field: "title" },
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Staff", read: true }] });`)
	write("doctypes/employee_line/employee_line.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee Line", isChild: true, fields: [
  { fieldname: "label", fieldtype: "Data", label: "Label" },
  { fieldname: "amount", fieldtype: "Currency", label: "Amount", permlevel: 1 },
] });`)
	write("doctypes/employee_bonus/employee_bonus.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee Bonus", isChild: true, fields: [
  { fieldname: "value", fieldtype: "Currency", label: "Value" },
] });`)
	for rel, src := range extra {
		write(rel, src)
	}
	return dir
}

func setupSEC02(t *testing.T, extra ...map[string]string) *Engine {
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
	e, err := New(ctx, Config{DSN: testDSN, Apps: []js.App{{Name: "fieldperm_test", Dir: sec02App(t, mergeFiles(extra))}}, Test: true})
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

func mergeFiles(all []map[string]string) map[string]string {
	out := map[string]string{}
	for _, m := range all {
		for k, v := range m {
			out[k] = v
		}
	}
	return out
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

func TestSEC02_Egress(t *testing.T) {
	t.Setenv("DDCORE_SECRET_KEY", "sec02-test-master-key")
	e := setupSEC02(t, map[string]string{
		"notifications/salary.notification.ts": `import { defineNotification } from "@ddcore/sdk";
export default defineNotification({ name: "salary", doctype: "Employee", event: "on_update",
  recipients() { return ["staff@x.com", "hr@x.com"] },
  desk: { title(doc) { return "Salary " + doc.salary }, message(doc) { return "Department " + doc.department } } });`,
	})
	ctx := context.Background()
	addWebhookFor(t, e, "Employee")
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.GetDoc("Employee", "Ana")
		if err != nil {
			return err
		}
		doc["salary"], doc["department"] = 150, "Finance"
		doc.Children("lines")[0]["amount"] = 6
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			return err
		}
		for _, f := range []Doc{
			{"file_name": "contract.pdf", "file_url": "/private/files/contract.pdf", "is_private": true,
				"attached_to_doctype": "Employee", "attached_to_name": "Ana", "attached_to_field": "contract"},
			{"file_name": "photo.png", "file_url": "/private/files/photo.png", "is_private": true,
				"attached_to_doctype": "Employee", "attached_to_name": "Ana"},
		} {
			fd, _ := c.NewDoc("File", f)
			if _, err := c.Insert(fd, SaveOpts{}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("webhook payload carries level 0 only", func(t *testing.T) {
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT payload::text AS payload FROM tab_webhook_delivery`)
		if err != nil || len(rows) != 1 {
			t.Fatalf("deliveries: %v %v", rows, err)
		}
		payload := db.Str(rows[0]["payload"])
		if strings.Contains(payload, "salary") || strings.Contains(payload, "bonuses") || strings.Contains(payload, "amount") {
			t.Fatalf("webhook leaked a restricted field: %s", payload)
		}
		if !strings.Contains(payload, "Finance") {
			t.Fatalf("webhook lost a level-0 field: %s", payload)
		}
	})

	t.Run("notification renders what the recipient may read", func(t *testing.T) {
		rows, err := db.Select(ctx, e.DB.Pool, `SELECT recipient, title FROM ddcore_notification`)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]string{}
		for _, r := range rows {
			got[db.Str(r["recipient"])] = db.Str(r["title"])
		}
		if got[sec02Staff] != "Salary undefined" || got[sec02HR] != "Salary 150" {
			t.Fatalf("titles = %v", got)
		}
	})

	t.Run("version diff", func(t *testing.T) {
		check := func(user string, wantSalary bool) {
			t.Helper()
			if err := e.Run(ctx, user, func(c *Ctx) error {
				rows, err := c.GetList("Version", ListArgs{Fields: []string{"name", "data"}, Filters: map[string]any{"ref_doctype": "Employee", "docname": "Ana"}})
				if err != nil {
					return err
				}
				if len(rows) != 1 {
					t.Fatalf("versions = %v", rows)
				}
				if _, ok := rows[0]["__version_ref"]; ok {
					t.Fatalf("internal column leaked: %v", rows[0])
				}
				listed := string(mustJSON(rows[0]["data"]))
				doc, err := c.GetDoc("Version", db.Str(rows[0]["name"]))
				if err != nil {
					return err
				}
				got := string(mustJSON(c.RedactDoc("Version", doc)["data"]))
				for _, data := range []string{listed, got} {
					if strings.Contains(data, "salary") != wantSalary || strings.Contains(data, "amount") != wantSalary {
						t.Fatalf("%s version data: %s", user, data)
					}
					if !strings.Contains(data, "Finance") {
						t.Fatalf("%s lost level-0 change: %s", user, data)
					}
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		}
		check(sec02Staff, false)
		check(sec02HR, true)
	})

	t.Run("export", func(t *testing.T) {
		if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
			var buf bytes.Buffer
			if _, err := c.Export(ExportArgs{Doctype: "Employee", Children: true, Attachments: true}, NewNDJSONSink(&buf)); err != nil {
				return err
			}
			out := buf.String()
			for _, leak := range []string{"salary", "amount", "bonuses", "contract.pdf"} {
				if strings.Contains(out, leak) {
					t.Fatalf("export leaked %s: %s", leak, out)
				}
			}
			if !strings.Contains(out, "Finance") || !strings.Contains(out, "photo.png") {
				t.Fatalf("export lost readable data: %s", out)
			}
			_, err := c.Export(ExportArgs{Doctype: "Employee", Fields: []string{"name", "salary"}}, NewNDJSONSink(&buf))
			wantPermissionError(t, "export of a restricted column", err)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Run(ctx, sec02HR, func(c *Ctx) error {
			var buf bytes.Buffer
			if _, err := c.Export(ExportArgs{Doctype: "Employee", Children: true, Attachments: true}, NewNDJSONSink(&buf)); err != nil {
				return err
			}
			if out := buf.String(); !strings.Contains(out, "salary") || !strings.Contains(out, "contract.pdf") {
				t.Fatalf("HR export incomplete: %s", out)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("print", func(t *testing.T) {
		render := func(user string) string {
			var html string
			if err := e.Run(ctx, user, func(c *Ctx) error {
				var err error
				html, err = c.PrintDoc("Employee", "Ana", "standard", "none", "en", print.PDFOptions{})
				return err
			}); err != nil {
				t.Fatal(err)
			}
			return html
		}
		if html := render(sec02Staff); strings.Contains(html, "Salary") || strings.Contains(html, "Amount") || strings.Contains(html, "Bonuses") {
			t.Fatalf("print leaked a restricted field: %s", html)
		}
		if html := render(sec02HR); !strings.Contains(html, "Salary") || !strings.Contains(html, "Amount") {
			t.Fatalf("HR print incomplete: %s", html)
		}
	})

	t.Run("files", func(t *testing.T) {
		if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
			restricted := map[string]any{"owner": "Administrator", "attached_to_doctype": "Employee", "attached_to_name": "Ana", "attached_to_field": "contract"}
			open := map[string]any{"owner": "Administrator", "attached_to_doctype": "Employee", "attached_to_name": "Ana"}
			if c.CanReadFile(restricted) || !c.CanReadFile(open) {
				t.Fatal("staff file access must follow the attachment field")
			}
			if restricted, canWrite := c.AttachmentFieldRestricted("Employee", "contract"); !restricted || canWrite {
				t.Fatal("staff may not upload into a restricted field")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if err := e.Run(ctx, sec02HR, func(c *Ctx) error {
			if !c.CanReadFile(map[string]any{"owner": "Administrator", "attached_to_doctype": "Employee", "attached_to_name": "Ana", "attached_to_field": "contract"}) {
				t.Fatal("HR reads the contract")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("ddcore.redact", func(t *testing.T) {
		if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
			doc, err := c.GetDoc("Employee", "Ana")
			if err != nil {
				return err
			}
			args, _ := json.Marshal(map[string]any{"doctype": "Employee", "doc": doc})
			rt, err := c.RT()
			if err != nil {
				return err
			}
			res, err := c.E.HostCall(rt, "redact", args)
			if err != nil {
				return err
			}
			out := string(mustJSON(res))
			if strings.Contains(out, "salary") || !strings.Contains(out, "Finance") {
				t.Fatalf("redact = %s", out)
			}
			if toFloat(doc["salary"]) != 150 {
				t.Fatal("redact must not change the caller's document")
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	})
}

func addWebhookFor(t *testing.T, e *Engine, doctype string) {
	t.Helper()
	if err := e.Run(context.Background(), "Administrator", func(c *Ctx) error {
		doc, err := c.NewDoc("Webhook", Doc{"url": "http://127.0.0.1:9/hook", "event_type": "Document", "webhook_doctype": doctype,
			"on_update": true, "secret": hookSecret, "max_attempts": 1})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("webhook: %v", err)
	}
}

// A scope on a Link the user cannot read is the framework's own condition: it
// must still narrow the list without the field check refusing the query.
func TestSEC02_ScopeOnRestrictedLink(t *testing.T) {
	e := setupSEC02(t)
	ctx := context.Background()
	if err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		for _, cc := range []string{"North", "South"} {
			d, _ := c.NewDoc("Cost Center", Doc{"title": cc})
			if _, err := c.Insert(d, SaveOpts{}); err != nil {
				return err
			}
		}
		doc, _ := c.GetDoc("Employee", "Ana")
		doc["cost_center"] = "North"
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			return err
		}
		other, _ := c.NewDoc("Employee", Doc{"title": "Bia", "cost_center": "South"})
		if _, err := c.Insert(other, SaveOpts{}); err != nil {
			return err
		}
		up, _ := c.NewDoc("User Permission", Doc{"user": sec02Staff, "allow": "Cost Center", "for_value": "North"})
		_, err := c.Insert(up, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, sec02Staff, func(c *Ctx) error {
		rows, err := c.GetList("Employee", ListArgs{Fields: []string{"name"}, OrderBy: "modified desc"})
		if err != nil {
			t.Fatalf("scoped list: %v", err)
		}
		if len(rows) != 1 || rows[0]["name"] != "Ana" {
			t.Fatalf("scope did not apply: %v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
