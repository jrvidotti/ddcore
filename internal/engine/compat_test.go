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
