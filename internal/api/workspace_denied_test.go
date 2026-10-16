package api

import (
	"os"
	"path/filepath"
	"testing"
)

// bootDenied is what boot tells auth: how many workspaces, and whether some
// were left out by the reader's roles.
func bootDenied(x *env, auth string) (int, any) {
	x.t.Helper()
	r := x.call("GET", "/api/boot", nil, auth)
	x.expect(r, 200, "")
	data, _ := r.Body["data"].(map[string]any)
	list, _ := data["workspaces"].([]any)
	return len(list), data["workspacesDenied"]
}

// The desk tells "no workspace is declared" from "none is yours" (#120).
func TestBoot_WorkspacesDeniedByRole(t *testing.T) {
	dir := testApp(t)
	// only Demo, which asks for Gestor, is left
	if err := os.Remove(filepath.Join(dir, "workspaces", "aberto.workspace.ts")); err != nil {
		t.Fatal(err)
	}
	x := setupSite(t, dir, false)
	if n, denied := bootDenied(x, "sid:"+x.sid("ze@x.com")); n != 0 || denied != true {
		t.Fatalf("a user without Gestor gets %d workspaces, workspacesDenied=%v", n, denied)
	}
	if n, denied := bootDenied(x, "sid:"+x.sid("ana@x.com")); n != 1 || denied != nil {
		t.Fatalf("a Gestor gets %d workspaces, workspacesDenied=%v", n, denied)
	}
}

func TestBoot_NoWorkspaceDeclaredIsNotDenied(t *testing.T) {
	dir := testApp(t)
	if err := os.RemoveAll(filepath.Join(dir, "workspaces")); err != nil {
		t.Fatal(err)
	}
	x := setupSite(t, dir, false)
	if n, denied := bootDenied(x, "sid:"+x.sid("ze@x.com")); n != 0 || denied != nil {
		t.Fatalf("a site without workspaces gives %d workspaces, workspacesDenied=%v", n, denied)
	}
}

// A workspace's entries follow what the reader may open: Settings is
// Gestor's alone, so a reader without it gets neither the entry, nor the
// shortcut, nor the heading left over nothing (#131).
func TestBoot_WorkspaceEntriesFollowTheReader(t *testing.T) {
	dir := testApp(t)
	src := `import { defineWorkspace } from "@ddcore/sdk";
export default defineWorkspace({ name: "Leitor", label: "Leitor",
  sidebar: [{ label: "Pessoas", doctype: "Pessoa" }, { label: "Admin" }, { label: "Settings", doctype: "Settings" }],
  shortcuts: [{ label: "Settings", doctype: "Settings" }],
  links: [{ label: "Admin", items: [{ label: "Settings", doctype: "Settings" }] }] });`
	if err := os.WriteFile(filepath.Join(dir, "workspaces", "leitor.workspace.ts"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	x := setupSite(t, dir, false)
	leitor := func(auth string) map[string]any {
		t.Helper()
		r := x.call("GET", "/api/boot", nil, auth)
		x.expect(r, 200, "")
		data, _ := r.Body["data"].(map[string]any)
		list, _ := data["workspaces"].([]any)
		for _, w := range list {
			if ws, _ := w.(map[string]any); ws["name"] == "Leitor" {
				return ws
			}
		}
		t.Fatalf("boot has no Leitor: %v", list)
		return nil
	}
	count := func(ws map[string]any, key string) int {
		l, _ := ws[key].([]any)
		return len(l)
	}
	ze := leitor("sid:" + x.sid("ze@x.com"))
	side, _ := ze["sidebar"].([]any)
	if len(side) != 1 || side[0].(map[string]any)["label"] != "Pessoas" || count(ze, "shortcuts") != 0 || count(ze, "links") != 0 {
		t.Fatalf("a reader without Gestor gets sidebar %v, shortcuts %v, links %v", side, ze["shortcuts"], ze["links"])
	}
	ana := leitor("sid:" + x.sid("ana@x.com"))
	if count(ana, "sidebar") != 3 || count(ana, "shortcuts") != 1 || count(ana, "links") != 1 {
		t.Fatalf("a Gestor gets sidebar %v, shortcuts %v, links %v", ana["sidebar"], ana["shortcuts"], ana["links"])
	}
}
