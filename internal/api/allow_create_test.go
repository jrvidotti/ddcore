package api

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// A DocType only server code creates: the meta tells the desk it cannot be
// created, even to Admin, and the REST insert is refused, while a server
// method still makes one.
func TestAllowCreateFalseRefusesTheRESTInsert(t *testing.T) {
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
	write("doctypes/transfer/transfer.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Transfer", allowCreate: false, description: "Made by the Transfer button",
  fields: [{ fieldname: "amount", fieldtype: "Int", label: "Amount" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true }] });`)
	write("services/transfers.ts", `import { whitelisted } from "@ddcore/sdk";
export const make = whitelisted(() => ddcore.newDoc("Transfer", { amount: 7 }).insert().id);`)
	x := setupApp(t, dir)
	admin := "sid:" + x.sid("Admin")

	r := x.call("GET", "/api/meta/Transfer", nil, admin)
	if r.Status != 200 {
		t.Fatalf("meta: %d %s", r.Status, r.Raw)
	}
	body, _ := r.Body["data"].(map[string]any)
	if perms := body["permissions"].(map[string]any); perms["create"] != false || perms["read"] != true {
		t.Fatalf("permissions: %v", perms)
	}
	if dt := body["doctype"].(map[string]any); dt["allowCreate"] != false || dt["description"] != "Made by the Transfer button" {
		t.Fatalf("doctype: %v", dt)
	}

	x.expect(x.call("POST", "/api/resource/Transfer", map[string]any{"amount": 1}, admin), 403, "PermissionError")

	if r := x.call("POST", "/api/method/demo.services.transfers.make", map[string]any{}, admin); r.Status != 200 {
		t.Fatalf("server code inserts: %d %s", r.Status, r.Raw)
	}
	x.asAdmin(func(c *engine.Ctx) error {
		n, err := c.Count("Transfer", nil)
		if err != nil {
			return err
		}
		if n != 1 {
			t.Fatalf("want the one Transfer server code made, got %d", n)
		}
		return nil
	})
}
