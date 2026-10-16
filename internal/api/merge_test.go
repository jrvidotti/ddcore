package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

func mergeAPIApp(t *testing.T) string {
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
export default defineApp({ name: "demo", title: "Merge API Test", roles: ["Gestor"] });`)
	write("doctypes/unit/unit.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Unit", idGeneration: { field: "title" }, allowRename: true,
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`)
	write("doctypes/employee/employee.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Employee", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "unit", fieldtype: "Link", label: "Unit", options: "Unit" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`)
	return dir
}

// #129: the desk previews a merge, then runs it through the rename endpoint.
func TestMergeAPI(t *testing.T) {
	x := setupApp(t, mergeAPIApp(t))
	ana := "sid:" + x.sid("ana@x.com")
	for _, body := range []map[string]any{{"title": "A"}, {"title": "B"}} {
		x.expect(x.call("POST", "/api/resource/Unit", body, ana), 200, "")
	}
	x.expect(x.call("POST", "/api/resource/Employee", map[string]any{"title": "E1", "unit": "A"}, ana), 200, "")

	// a plain rename into an existing id is still refused
	x.expect(x.call("POST", "/api/resource/Unit/A/rename", map[string]any{"id": "B"}, ana), 409, "DuplicateEntryError")

	r := x.call("POST", "/api/resource/Unit/A/merge_preview", map[string]any{"id": "B"}, ana)
	x.expect(r, 200, "")
	p := r.Body["data"].(map[string]any)
	links := p["links"].([]any)
	if len(links) != 1 || links[0].(map[string]any)["doctype"] != "Employee" || links[0].(map[string]any)["count"] != float64(1) {
		t.Fatalf("preview = %s", r.Raw)
	}
	if b := p["blockers"].([]any); len(b) != 0 {
		t.Fatalf("blockers = %v", b)
	}
	x.expect(x.call("POST", "/api/resource/Unit/A/merge_preview", map[string]any{"id": "A"}, ana), 417, "ValidationError")

	r = x.call("POST", "/api/resource/Unit/A/rename", map[string]any{"id": "B", "merge": true}, ana)
	x.expect(r, 200, "")
	if r.Body["data"] != "B" {
		t.Fatalf("merge answered %s", r.Raw)
	}
	x.asAdmin(func(c *engine.Ctx) error {
		if ok, _ := c.Exists("Unit", "A"); ok {
			t.Fatal("A survived the merge")
		}
		e, err := c.GetDoc("Employee", "E1")
		if err != nil {
			return err
		}
		if e.Str("unit") != "B" {
			t.Fatalf("Employee still on %q", e.Str("unit"))
		}
		return nil
	})
}
