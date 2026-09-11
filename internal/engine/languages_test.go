package engine

import "testing"

// TestLanguageName pins the autonyms — each language written in itself, which is
// what makes a language picker usable by someone who cannot read the current one.
func TestLanguageName(t *testing.T) {
	cases := []struct{ code, want string }{
		{"en", "English"},
		// CLDR names Brazilian Portuguese simply "português": it is the default
		// content for `pt`, so the region carries no separate name. Where a
		// region *is* distinct, it shows up — see es-MX below. The first letter
		// is capitalised so the picker reads evenly; the rest is left as CLDR
		// writes it.
		{"pt-BR", "Português"},
		{"fr", "Français"},
		{"es-MX", "Español de México"},
		// unknown or unparseable: the code itself, which beats a blank entry
		{"xx", "xx"},
		{"not a tag", "not a tag"},
		{"", ""},
	}
	for _, c := range cases {
		if got := LanguageName(c.code); got != c.want {
			t.Errorf("LanguageName(%q) = %q, want %q", c.code, got, c.want)
		}
	}
}
