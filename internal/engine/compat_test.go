package engine

import "testing"

func TestAppRange(t *testing.T) {
	for core, want := range map[string]string{
		"v0.15.0":              ">=0.15.0 <0.16.0",
		"v0.15.2-3-gabc-dirty": ">=0.15.0 <0.16.0",
		"1.2.3":                ">=1.2.0 <1.3.0",
		"0.1.0":                "",
		"dev":                  "",
	} {
		got, ok := AppRange(core)
		if got != want || ok != (want != "") {
			t.Errorf("AppRange(%q) = %q, %v; want %q", core, got, ok, want)
		}
		if ok {
			if _, err := parseRange(got); err != nil {
				t.Errorf("AppRange(%q) = %q does not parse: %v", core, got, err)
			}
		}
	}
}

// An app whose range reaches below 0.17.0 was written when the document key
// was `name`; a 0.17 binary says so at load, and an older build does not.
func TestPredatesIDKey(t *testing.T) {
	snap := &Snapshot{Apps: map[string]*AppMeta{
		"core":   {Name: "core"},
		"old":    {Name: "old", Ddcore: ">=0.14.0 <1.0.0"},
		"open":   {Name: "open", Ddcore: "<1.0.0"},
		"new":    {Name: "new", Ddcore: ">=0.17.0 <0.18.0"},
		"caret":  {Name: "caret", Ddcore: "^0.17"},
		"broken": {Name: "broken", Ddcore: "whatever"},
	}}
	got := predatesIDKey(snap, "v0.17.0")
	if len(got) != 2 || got[0] != "old" || got[1] != "open" {
		t.Fatalf("on 0.17.0: %v, want [old open]", got)
	}
	for _, core := range []string{"v0.16.0-4-gabc123", "dev"} {
		if got := predatesIDKey(snap, core); got != nil {
			t.Errorf("on %s: %v, want nothing", core, got)
		}
	}
}
