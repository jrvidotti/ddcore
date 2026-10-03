package engine

import (
	"strings"
	"testing"
)

func TestWorkspaceSpaceError(t *testing.T) {
	ok := map[string]map[string]any{
		"Back Office": {"space": "platform", "sidebar": []any{map[string]any{"label": "A", "space": "tenant"}, map[string]any{"label": "B"}}},
		"Plain":       {"sidebar": []any{}},
	}
	if err := workspaceSpaceError(ok); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]map[string]any{
		{"Back Office": {"space": "tenants"}},
		{"Back Office": {"numberCards": []any{map[string]any{"name": "x", "space": "Platform"}}}},
	} {
		err := workspaceSpaceError(bad)
		if err == nil || !strings.Contains(err.Error(), `workspace "Back Office"`) {
			t.Fatalf("a bad space passes: %v", err)
		}
	}
}

func TestWorkspaceForSpaceLeavesTheSnapshotAlone(t *testing.T) {
	ws := map[string]any{"name": "W", "shortcuts": []any{
		map[string]any{"label": "P", "space": "platform"},
		map[string]any{"label": "T", "space": "tenant"},
	}}
	got := WorkspaceForSpace(ws, SpaceTenant)
	if l := got["shortcuts"].([]any); len(l) != 1 || l[0].(map[string]any)["label"] != "T" {
		t.Fatalf("a tenant gets %v", l)
	}
	if len(ws["shortcuts"].([]any)) != 2 {
		t.Fatalf("the snapshot was changed: %v", ws)
	}
	if WorkspaceForSpace(map[string]any{"space": "platform"}, SpaceTenant) != nil {
		t.Fatal("a tenant gets the platform's workspace")
	}
	if WorkspaceForSpace(map[string]any{"space": "platform"}, "") == nil {
		t.Fatal("without tenancy a workspace is hidden")
	}
}
