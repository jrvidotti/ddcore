package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestIconError(t *testing.T) {
	reg := meta.NewRegistry()
	if err := reg.Add(&meta.DocType{Name: "Payout", Icon: "wallet"}); err != nil {
		t.Fatal(err)
	}
	ok := map[string]map[string]any{
		"Payments": {"icon": "landmark", "sidebar": []any{
			map[string]any{"label": "Refunds", "icon": "undo-2"},
			map[string]any{"label": "Records"},
			map[string]any{"label": "Alerts", "icon": "triangle-alert"},
		}},
	}
	if err := iconError(reg, ok); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		ws   map[string]map[string]any
		dt   string
		want string
	}{
		{ws: map[string]map[string]any{"Payments": {"icon": "piggy"}}, want: `workspace "Payments": icon "piggy"`},
		{ws: map[string]map[string]any{"Payments": {"sidebar": []any{map[string]any{"label": "A"}, map[string]any{"label": "B", "icon": "piggy"}}}}, want: `workspace "Payments", sidebar[1]: icon "piggy"`},
		{ws: map[string]map[string]any{"Payments": {"links": []any{map[string]any{"label": "G", "items": []any{map[string]any{"label": "B", "icon": "piggy"}}}}}}, want: `workspace "Payments", links[0], items[0]: icon "piggy"`},
		{dt: "piggy", want: `DocType "Ledger": icon "piggy"`},
	} {
		r := meta.NewRegistry()
		if c.dt != "" {
			_ = r.Add(&meta.DocType{Name: "Ledger", Icon: c.dt})
		}
		err := iconError(r, c.ws)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("want %q, got %v", c.want, err)
		}
	}
}
