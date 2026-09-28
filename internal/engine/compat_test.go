package engine

import (
	"strings"
	"testing"
)

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

// The image tag is the minor series, which the release workflow publishes as
// :X.Y and moves to each patch, so it always sits inside AppRange's range.
func TestImageTag(t *testing.T) {
	for core, want := range map[string]string{
		"v0.21.0":              "0.21",
		"v0.21.3-2-gabc-dirty": "0.21",
		"1.2.3":                "1.2",
		"0.1.0":                "",
		"dev":                  "",
	} {
		if got, ok := ImageTag(core); got != want || ok != (want != "") {
			t.Errorf("ImageTag(%q) = %q, %v; want %q", core, got, ok, want)
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

// The site's range in ddcore.json is checked like an app's: parsed on every
// build, enforced on a release, and reported in the same joined error.
func TestSiteCoreRange(t *testing.T) {
	apps := &Snapshot{Apps: map[string]*AppMeta{"core": {Name: "core"}, "shop": {Name: "shop"}}}
	if _, err := checkCoreCompat(apps, ">=0.20.0 <0.21.0", "v0.20.2"); err != nil {
		t.Fatalf("in range: %v", err)
	}
	_, err := checkCoreCompat(apps, ">=0.20.0 <0.21.0", "v0.21.0")
	if err == nil || !strings.Contains(err.Error(), "site requires ddcore >=0.20.0 <0.21.0 (ddcore.json), but this binary is 0.21.0") {
		t.Fatalf("out of range: %v", err)
	}
	if skipped, err := checkCoreCompat(apps, ">=9.0.0", "dev"); err != nil || !skipped {
		t.Fatalf("dev build: skipped=%v err=%v", skipped, err)
	}
	if _, err := checkCoreCompat(apps, ">= 0.20.0", "dev"); err == nil || !strings.Contains(err.Error(), "ddcore.json declares an invalid ddcore range") {
		t.Fatalf("invalid range must be refused even on a dev build: %v", err)
	}

	// An app's own range still holds on top of the site's, in one error.
	both := &Snapshot{Apps: map[string]*AppMeta{"shop": {Name: "shop", Ddcore: "<0.20.0"}}}
	_, err = checkCoreCompat(both, ">=0.21.0", "v0.20.2")
	if err == nil || !strings.Contains(err.Error(), "site requires ddcore") || !strings.Contains(err.Error(), "app shop requires ddcore") {
		t.Fatalf("expected both problems in one error: %v", err)
	}
}
