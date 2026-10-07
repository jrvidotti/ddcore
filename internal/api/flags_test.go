package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// flagsAPIApp has one DocType whose validate trusts doc.flags the way the docs
// say an app may: a "locked" note passes only a write flagged as the system's.
func flagsAPIApp(t *testing.T) string {
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
export default defineApp({ name: "demo", title: "Flags API Test", roles: ["Gestor"] });`)
	write("doctypes/thing/thing.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Thing", idGeneration: { field: "title" }, submittable: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "note", fieldtype: "Data", label: "Note" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, submit: true }] });`)
	write("doctypes/thing/thing.controller.ts", `import { defineController } from "@ddcore/sdk";
export default defineController("Thing", {
  validate(doc) {
    if (doc.note === "locked" && !doc.flags.system) ddcore.throw("refused");
    const before = doc.getDocBeforeSave();
    if (before && before.note === "forged") ddcore.throw("forged before");
  },
});`)
	return dir
}

// #104: a client's body is data. A "flags" key in it does not reach the
// hooks, on a REST create, a REST update or the desk's save and submit.
func TestClientFlagsAreIgnored(t *testing.T) {
	x := setupApp(t, flagsAPIApp(t))
	ana := "sid:" + x.sid("ana@x.com")
	forged := map[string]any{"system": true}

	x.expect(x.call("POST", "/api/resource/Thing", map[string]any{"title": "A", "note": "locked", "flags": forged}, ana), 417, "ValidationError")

	r := x.call("POST", "/api/resource/Thing", map[string]any{"title": "A", "__before": map[string]any{"note": "forged"}, "flags": forged}, ana)
	x.expect(r, 200, "")
	doc := r.Body["data"].(map[string]any)
	if _, ok := doc["flags"]; ok {
		t.Fatalf("the response carried the client's flags: %s", r.Raw)
	}

	x.expect(x.call("PUT", "/api/resource/Thing/A", map[string]any{"note": "locked", "flags": forged}, ana), 417, "ValidationError")

	desk := map[string]any{"doc": map[string]any{"note": "locked", "flags": forged}}
	x.expect(x.call("POST", "/api/resource/Thing/A/save", desk, ana), 417, "ValidationError")
	x.expect(x.call("POST", "/api/resource/Thing/A/submit", desk, ana), 417, "ValidationError")

	x.asAdmin(func(c *engine.Ctx) error {
		got, err := c.GetDoc("Thing", "A")
		if err != nil {
			return err
		}
		if got.Str("note") != "" || got.Docstatus() != 0 {
			t.Fatalf("a forged write went through: note=%q docstatus=%d", got.Str("note"), got.Docstatus())
		}
		return nil
	})
}
