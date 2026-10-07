package api

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/engine"
)

// tenantAccessApp is the test app with a shared DocType the platform keeps
// (#108), a tenant DocType that links to it and a workspace that opens it.
func tenantAccessApp(t *testing.T) string {
	t.Helper()
	dir := testApp(t)
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("doctypes/consulta/consulta.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Consulta", shared: true, tenantAccess: "server", idGeneration: { field: "documento" }, titleField: "nome",
  fields: [{ fieldname: "documento", fieldtype: "Data", label: "Documento", reqd: true },
           { fieldname: "nome", fieldtype: "Data", label: "Nome" },
           { fieldname: "anexo", fieldtype: "Attach", label: "Anexo" }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, report: true, export: true }] });`)
	w("doctypes/ficha/ficha.doctype.ts", `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Ficha",
  fields: [{ fieldname: "consulta", fieldtype: "Link", label: "Consulta", options: "Consulta" }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }] });`)
	w("workspaces/cadastro.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Cadastro", label: "Cadastro",
  sidebar: [{ label: "Consultas", doctype: "Consulta" }, { label: "Fichas", doctype: "Ficha" }] });`)
	return dir
}

func TestTenantAccess_ATenantClientNeverReachesIt(t *testing.T) {
	x := setupTenantsWith(t, tenantAccessApp(t))
	admin, chefe := "sid:"+x.sid("Admin"), "sid:"+x.sid(alfaAdmin)
	x.asAdmin(func(c *engine.Ctx) error {
		d, _ := c.NewDoc("Consulta", engine.Doc{"documento": "123", "nome": "Maria da Silva"})
		_, err := c.Insert(d, engine.SaveOpts{})
		return err
	})
	refused := func(r resp) {
		t.Helper()
		x.expect(r, 403, "PermissionError")
		if !strings.Contains(r.Raw, "kept by the platform") {
			t.Fatalf("the refusal does not say why: %s", r.Raw)
		}
	}

	// the tenant's System Manager, who holds the role
	refused(x.call("GET", "/api/resource/Consulta", nil, chefe))
	refused(x.call("GET", "/api/resource/Consulta/123", nil, chefe))
	refused(x.call("GET", "/api/meta/Consulta", nil, chefe))
	refused(x.call("POST", "/api/resource/Consulta", map[string]any{"documento": "999"}, chefe))
	refused(x.uploadToField(chefe, "Consulta", "123", "anexo", "1"))
	r := x.call("GET", "/api/boot", nil, chefe)
	x.expect(r, 200, "")
	if dts, _ := r.Body["data"].(map[string]any)["doctypes"].(map[string]any); dts["Consulta"] != nil {
		t.Fatal("a tenant's boot lists Consulta")
	}
	if got := bootWorkspaces(x, chefe)["Cadastro"]; got != "Fichas |  | " {
		t.Fatalf("a tenant's workspace = %q", got)
	}
	// a Link to it saves (server code checks it exists) but shows no title
	r = x.call("POST", "/api/resource/Ficha", map[string]any{"consulta": "123"}, chefe)
	x.expect(r, 200, "")
	id := r.Body["data"].(map[string]any)["id"].(string)
	r = x.call("GET", "/api/resource/Ficha/"+id, nil, chefe)
	x.expect(r, 200, "")
	if titles, _ := r.Body["data"].(map[string]any)["_linkTitles"].(map[string]any); titles["Consulta"] != nil {
		t.Fatalf("a tenant reads Consulta's titles: %v", titles)
	}
	if strings.Contains(r.Raw, "Maria") {
		t.Fatalf("Consulta's data reached a tenant: %s", r.Raw)
	}
	refused(x.call("GET", "/api/search/link-titles?doctype=Consulta&ids=123", nil, chefe))

	// the operator in the platform space works with it as before
	x.expect(x.call("GET", "/api/resource/Consulta/123", nil, admin), 200, "")
	x.expect(x.call("GET", "/api/meta/Consulta", nil, admin), 200, "")
	if got := bootWorkspaces(x, admin)["Cadastro"]; got != "Consultas,Fichas |  | " {
		t.Fatalf("the platform's workspace = %q", got)
	}
}
