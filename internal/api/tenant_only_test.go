package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// tenantOnlyApp is the test app with a DocType that lives inside a tenant
// (#105), a report on it and a workspace that points at both without naming
// a space.
func tenantOnlyApp(t *testing.T) string {
	t.Helper()
	dir := testApp(t)
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("doctypes/folha/folha.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Folha", space: "tenant",
  fields: [{ fieldname: "mes", fieldtype: "Data", label: "Mes", reqd: true }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }] });`)
	w("reports/folhas.report.ts", `import { defineReport } from "@ddcore/sdk";
export default defineReport({ name: "Folhas", refDoctype: "Folha", filters: [],
  execute() { return { columns: [{ fieldname: "id", label: "Id" }], rows: ddcore.db.getList("Folha", { fields: ["id"] }) }; } });`)
	w("workspaces/rh.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "RH", label: "RH",
  sidebar: [{ label: "Folhas", doctype: "Folha" }, { label: "Pedidos", doctype: "Pedido" }],
  shortcuts: [{ label: "Relatorio", report: "Folhas" }],
  numberCards: [{ name: "folhas", label: "Folhas", doctype: "Folha" }] });`)
	return dir
}

func TestTenantOnly_ThePlatformNeitherListsNorOpensIt(t *testing.T) {
	x := setupTenantsWith(t, tenantOnlyApp(t))
	admin, chefe := "sid:"+x.sid("Admin"), "sid:"+x.sid(alfaAdmin)

	boot := func(auth string) (doctypes, reports map[string]any) {
		r := x.call("GET", "/api/boot", nil, auth)
		x.expect(r, 200, "")
		data, _ := r.Body["data"].(map[string]any)
		doctypes, _ = data["doctypes"].(map[string]any)
		reports, _ = data["reports"].(map[string]any)
		return
	}
	refused := func(r resp) {
		t.Helper()
		x.expect(r, 403, "PermissionError")
		if !strings.Contains(r.Raw, "lives inside a tenant") {
			t.Fatalf("the refusal does not say why: %s", r.Raw)
		}
	}

	// the platform space
	dts, reps := boot(admin)
	if _, ok := dts["Folha"]; ok {
		t.Fatal("the platform's boot lists Folha")
	}
	if _, ok := reps["Folhas"]; ok {
		t.Fatal("the platform's boot lists a report on Folha")
	}
	if _, ok := dts["Site Tenant"]; !ok {
		t.Fatal("the platform's boot misses Site Tenant")
	}
	if got := bootWorkspaces(x, admin)["RH"]; got != "Pedidos |  | " {
		t.Fatalf("the platform's RH workspace = %q", got)
	}
	refused(x.call("GET", "/api/meta/Folha", nil, admin))
	refused(x.call("POST", "/api/resource/Folha", map[string]any{"mes": "2026-10"}, admin))
	refused(x.call("GET", "/api/resource/Folha", nil, admin))
	x.expect(x.call("GET", "/api/workspace/RH/card/folhas", nil, admin), 404, "")

	// inside a tenant it is an ordinary DocType, and Site Tenant is the one
	// left out
	dts, reps = boot(chefe)
	if _, ok := dts["Folha"]; !ok {
		t.Fatal("a tenant's boot misses Folha")
	}
	if _, ok := reps["Folhas"]; !ok {
		t.Fatal("a tenant's boot misses the report on Folha")
	}
	if _, ok := dts["Site Tenant"]; ok {
		t.Fatal("a tenant's boot lists Site Tenant")
	}
	if got := bootWorkspaces(x, chefe)["RH"]; got != "Folhas,Pedidos | Relatorio | Folhas" {
		t.Fatalf("a tenant's RH workspace = %q", got)
	}
	x.expect(x.call("GET", "/api/meta/Folha", nil, chefe), 200, "")
	x.expect(x.call("POST", "/api/resource/Folha", map[string]any{"mes": "2026-10"}, chefe), 200, "")
	x.expect(x.call("GET", "/api/workspace/RH/card/folhas", nil, chefe), 200, "")

	// the operator who enters the tenant works there
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "alfa"}, admin), 200, "")
	if dts, _ := boot(admin); dts["Folha"] == nil {
		t.Fatal("the operator inside alfa misses Folha")
	}
	x.expect(x.call("POST", "/api/resource/Folha", map[string]any{"mes": "2026-11"}, admin), 200, "")
}
