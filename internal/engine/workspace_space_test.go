package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
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
	st := &State{Meta: meta.NewRegistry()}
	got := st.WorkspaceForSpace(ws, SpaceTenant)
	if l := got["shortcuts"].([]any); len(l) != 1 || l[0].(map[string]any)["label"] != "T" {
		t.Fatalf("a tenant gets %v", l)
	}
	if len(ws["shortcuts"].([]any)) != 2 {
		t.Fatalf("the snapshot was changed: %v", ws)
	}
	if st.WorkspaceForSpace(map[string]any{"space": "platform"}, SpaceTenant) != nil {
		t.Fatal("a tenant gets the platform's workspace")
	}
	if st.WorkspaceForSpace(map[string]any{"space": "platform"}, "") == nil {
		t.Fatal("without tenancy a workspace is hidden")
	}
}

// An item that names no space takes the one of what it opens: a tenant-only
// DocType, or a report on one, is a tenant's item (#105).
func TestWorkspaceItemsTakeTheirDocTypesSpace(t *testing.T) {
	reg := meta.NewRegistry()
	for _, d := range []*meta.DocType{{Name: "Folha", TenantOwned: true, TenantOnly: true}, {Name: "Pais"}} {
		if err := reg.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	st := &State{Meta: reg, Snap: &Snapshot{Reports: map[string]map[string]any{"Folhas": {"refDoctype": "Folha"}}}}
	ws := map[string]any{
		"sidebar": []any{
			map[string]any{"label": "Folha", "doctype": "Folha"},
			map[string]any{"label": "Pinned", "doctype": "Folha", "space": "platform"},
			map[string]any{"label": "Folhas", "report": "Folhas"},
			map[string]any{"label": "Pais", "doctype": "Pais"},
		},
		"numberCards": []any{map[string]any{"name": "n", "refDoctype": "Folha"}},
		"links": []any{
			map[string]any{"label": "RH", "items": []any{map[string]any{"label": "Folha", "doctype": "Folha"}}},
			map[string]any{"label": "Geral", "items": []any{map[string]any{"label": "Folha", "doctype": "Folha"}, map[string]any{"label": "Pais", "doctype": "Pais"}}},
		},
	}
	labels := func(list any) []string {
		var out []string
		for _, it := range list.([]any) {
			out = append(out, it.(map[string]any)["label"].(string))
		}
		return out
	}
	p := st.WorkspaceForSpace(ws, SpacePlatform)
	if got := strings.Join(labels(p["sidebar"]), ","); got != "Pinned,Pais" {
		t.Fatalf("platform sidebar = %s", got)
	}
	if len(p["numberCards"].([]any)) != 0 {
		t.Fatal("the platform gets a card on a tenant-only DocType")
	}
	groups := p["links"].([]any)
	if len(groups) != 1 || strings.Join(labels(groups[0].(map[string]any)["items"]), ",") != "Pais" {
		t.Fatalf("platform links = %v", groups)
	}
	tn := st.WorkspaceForSpace(ws, SpaceTenant)
	if got := strings.Join(labels(tn["sidebar"]), ","); got != "Folha,Folhas,Pais" {
		t.Fatalf("tenant sidebar = %s", got)
	}
	if len(tn["links"].([]any)) != 2 {
		t.Fatalf("tenant links = %v", tn["links"])
	}
	if got := st.WorkspaceForSpace(ws, ""); len(got["sidebar"].([]any)) != 4 {
		t.Fatal("without tenancy an item is hidden")
	}
}

// A heading over items of the other space goes with them (#131).
func TestWorkspaceForSpaceDropsAHeadingOverNothing(t *testing.T) {
	ws := map[string]any{"sidebar": []any{
		map[string]any{"label": "Both", "doctype": "Both"},
		map[string]any{"label": "Tenants"},
		map[string]any{"label": "Site Tenant", "doctype": "Site Tenant", "space": "platform"},
	}}
	got := (&State{Meta: meta.NewRegistry()}).WorkspaceForSpace(ws, SpaceTenant)
	if l := got["sidebar"].([]any); len(l) != 1 || l[0].(map[string]any)["label"] != "Both" {
		t.Fatalf("a tenant's sidebar = %v", l)
	}
}

// Boot leaves out what the reader may not open: a DocType that is not in its
// doctypes, a report that is not in its reports, and a heading or a links
// group over nothing left (#131).
func TestWorkspaceForReader(t *testing.T) {
	doctypes := map[string]any{"Invoice": map[string]any{}}
	reports := map[string]any{"Sales Report": map[string]any{}}
	ws := map[string]any{
		"name": "Sales",
		"sidebar": []any{
			map[string]any{"label": "Overview", "route": "/app/workspace/Sales"},
			map[string]any{"label": "Invoices", "doctype": "Invoice"},
			map[string]any{"label": "Admin"},
			map[string]any{"label": "Payouts", "doctype": "Payout"},
			map[string]any{"label": "Payout Forecast", "report": "Payout Forecast"},
			map[string]any{"label": "Reports"},
			map[string]any{"label": "Sales Report", "report": "Sales Report"},
			map[string]any{"label": "Payout Forecast", "report": "Payout Forecast"},
			map[string]any{"label": "Empty"},
		},
		"shortcuts": []any{
			map[string]any{"label": "Payouts", "doctype": "Payout"},
			map[string]any{"label": "Invoices", "doctype": "Invoice"},
		},
		"numberCards": []any{
			map[string]any{"name": "a", "doctype": "Payout"},
			map[string]any{"name": "b", "refDoctype": "Payout"},
			map[string]any{"name": "c", "doctype": "Invoice"},
			map[string]any{"name": "d"},
		},
		"charts": []any{
			map[string]any{"name": "a", "refDoctype": "Payout"},
			map[string]any{"name": "b"},
		},
		"links": []any{
			map[string]any{"label": "Admin", "items": []any{map[string]any{"label": "Payouts", "doctype": "Payout"}}},
			map[string]any{"label": "Mixed", "items": []any{
				map[string]any{"label": "Payouts", "doctype": "Payout"},
				map[string]any{"label": "Invoices", "doctype": "Invoice"},
			}},
		},
	}
	labels := func(list any, key string) string {
		var out []string
		for _, it := range list.([]any) {
			out = append(out, it.(map[string]any)[key].(string))
		}
		return strings.Join(out, ",")
	}
	got := WorkspaceForReader(ws, doctypes, reports)
	for _, c := range []struct{ list, key, want string }{
		{"sidebar", "label", "Overview,Invoices,Reports,Sales Report,Empty"},
		{"shortcuts", "label", "Invoices"},
		{"numberCards", "name", "c,d"},
		{"charts", "name", "b"},
		{"links", "label", "Mixed"},
	} {
		if s := labels(got[c.list], c.key); s != c.want {
			t.Errorf("%s = %s, want %s", c.list, s, c.want)
		}
	}
	if s := labels(got["links"].([]any)[0].(map[string]any)["items"], "label"); s != "Invoices" {
		t.Errorf("the mixed group keeps %s", s)
	}
	if len(ws["sidebar"].([]any)) != 9 || len(ws["links"].([]any)[1].(map[string]any)["items"].([]any)) != 2 {
		t.Fatalf("the snapshot was changed: %v", ws)
	}
}
