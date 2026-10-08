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
