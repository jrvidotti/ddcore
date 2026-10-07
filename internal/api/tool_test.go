package api

import (
	"os"
	"path/filepath"
	"testing"
)

// A tool Single (#114): the meta tells the desk it is one, and the REST save
// is refused, to Admin too.
func TestToolSingleREST(t *testing.T) {
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
export default defineApp({ name: "demo", title: "Demo", roles: ["Gestor"] });`)
	write("doctypes/link_generator/link_generator.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Link Generator", label: "Link Generator", isSingle: true, tool: true,
  fields: [{ fieldname: "cpf", fieldtype: "Data", label: "CPF" }],
  permissions: [{ role: "Gestor", read: true }] });`)
	x := setupApp(t, dir)
	admin := "sid:" + x.sid("Admin")

	r := x.call("GET", "/api/meta/Link Generator", nil, admin)
	if r.Status != 200 {
		t.Fatalf("meta: %d %s", r.Status, r.Raw)
	}
	if dt := r.Body["data"].(map[string]any)["doctype"].(map[string]any); dt["tool"] != true {
		t.Fatalf("doctype: %v", dt)
	}
	path := "/api/resource/Link Generator/singleton"
	r = x.call("GET", path, nil, admin)
	if r.Status != 200 {
		t.Fatalf("read: %d %s", r.Status, r.Raw)
	}
	doc := r.Body["data"].(map[string]any)
	doc["cpf"] = "52998224725"
	x.expect(x.call("PUT", path, doc, admin), 417, "ValidationError")
}
