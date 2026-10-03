package api

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// spacesApp is the test app with a workspace whose items name their space,
// and a workspace of the platform's alone.
func spacesApp(t *testing.T) string {
	t.Helper()
	dir := testApp(t)
	w := func(rel, src string) {
		os.MkdirAll(filepath.Join(dir, filepath.Dir(rel)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, rel), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	w("workspaces/espacos.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Espacos", label: "Espacos",
  sidebar: [
    { label: "Contas", doctype: "Site Tenant", space: "platform" },
    { label: "Pessoas", doctype: "Pessoa", space: "tenant" },
    { label: "Pedidos", doctype: "Pedido" },
  ],
  shortcuts: [
    { label: "Nova conta", doctype: "Site Tenant", space: "platform" },
    { label: "Nova pessoa", doctype: "Pessoa", space: "tenant" },
  ],
  numberCards: [
    { name: "contas", label: "Contas", doctype: "Site Tenant", space: "platform" },
    { name: "pessoas", label: "Pessoas", doctype: "Pessoa", space: "tenant" },
  ] });`)
	w("workspaces/plataforma.workspace.ts", `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Plataforma", label: "Plataforma", space: "platform",
  sidebar: [{ label: "Contas", doctype: "Site Tenant" }],
  numberCards: [{ name: "contas", label: "Contas", doctype: "Site Tenant" }] });`)
	return dir
}

// bootWorkspaces is what boot tells auth of the workspaces: each one's name,
// then the labels of its sidebar, shortcuts and number cards.
func bootWorkspaces(x *env, auth string) map[string]string {
	x.t.Helper()
	r := x.call("GET", "/api/boot", nil, auth)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	list, _ := data["workspaces"].([]any)
	out := map[string]string{}
	for _, wsAny := range list {
		ws, _ := wsAny.(map[string]any)
		var parts []string
		for _, key := range []string{"sidebar", "shortcuts", "numberCards"} {
			items, _ := ws[key].([]any)
			var labels []string
			for _, it := range items {
				labels = append(labels, fmt.Sprint(it.(map[string]any)["label"]))
			}
			parts = append(parts, strings.Join(labels, ","))
		}
		out[fmt.Sprint(ws["name"])] = strings.Join(parts, " | ")
	}
	return out
}

func names(m map[string]string) string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func TestWorkspaceSpace_BootShowsEachSpaceItsOwn(t *testing.T) {
	x := setupTenantsWith(t, spacesApp(t))
	admin := "sid:" + x.sid("Admin")

	platform := bootWorkspaces(x, admin)
	if got := platform["Espacos"]; got != "Contas,Pedidos | Nova conta | Contas" {
		t.Fatalf("the platform sees %q", got)
	}
	if _, ok := platform["Plataforma"]; !ok {
		t.Fatalf("the platform misses its own workspace: %s", names(platform))
	}

	tenant := bootWorkspaces(x, "sid:"+x.sid(alfaAdmin))
	if got := tenant["Espacos"]; got != "Pessoas,Pedidos | Nova pessoa | Pessoas" {
		t.Fatalf("a tenant sees %q", got)
	}
	if _, ok := tenant["Plataforma"]; ok {
		t.Fatalf("a tenant sees the platform's workspace: %s", names(tenant))
	}

	// the operator entering a tenant sees what the tenant sees, and back
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": "alfa"}, admin), 200, "")
	if got := bootWorkspaces(x, admin)["Espacos"]; got != "Pessoas,Pedidos | Nova pessoa | Pessoas" {
		t.Fatalf("the operator inside alfa sees %q", got)
	}
	x.expect(x.call("POST", "/api/tenant/enter", map[string]any{"tenant": ""}, admin), 200, "")
	if got := bootWorkspaces(x, admin)["Espacos"]; got != "Contas,Pedidos | Nova conta | Contas" {
		t.Fatalf("the operator back in the platform sees %q", got)
	}
}

func TestWorkspaceSpace_CardsAnswerOnlyInTheirSpace(t *testing.T) {
	x := setupTenantsWith(t, spacesApp(t))
	admin, tenant := "sid:"+x.sid("Admin"), "sid:"+x.sid(alfaAdmin)
	x.expect(x.call("GET", "/api/workspace/Espacos/card/contas", nil, admin), 200, "")
	x.expect(x.call("GET", "/api/workspace/Espacos/card/pessoas", nil, admin), 404, "")
	x.expect(x.call("GET", "/api/workspace/Espacos/card/pessoas", nil, tenant), 200, "")
	x.expect(x.call("GET", "/api/workspace/Espacos/card/contas", nil, tenant), 404, "")
	x.expect(x.call("GET", "/api/workspace/Plataforma/card/contas", nil, admin), 200, "")
	x.expect(x.call("GET", "/api/workspace/Plataforma/card/contas", nil, tenant), 404, "")
}

func TestWorkspaceSpace_IgnoredWithoutTenancy(t *testing.T) {
	x := setupSite(t, spacesApp(t), false)
	all := bootWorkspaces(x, "sid:"+x.sid("Admin"))
	if got := all["Espacos"]; got != "Contas,Pessoas,Pedidos | Nova conta,Nova pessoa | Contas,Pessoas" {
		t.Fatalf("without tenancy the workspace shows %q", got)
	}
	if _, ok := all["Plataforma"]; !ok {
		t.Fatalf("without tenancy a platform workspace is hidden: %s", names(all))
	}
}
