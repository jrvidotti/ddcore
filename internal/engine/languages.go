package engine

import (
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// languageField is the one field whose options are the site's languages.
const (
	languageDocType   = "User"
	languageFieldname = "language"
)

// applyLanguageOptions turns User.language into a picker over the languages this
// site actually serves.
//
// The list cannot be declared in the DocType's TypeScript: it is whichever
// translations/<lang>.csv happen to exist across the core and the apps, which is
// only known once they are loaded. So it is filled here, into the registry —
// not at the API border — because a Select validates against the registry's
// options when a document is saved.
//
// The labels are autonyms: each language written in itself ("English",
// "Português"). That is the convention for a language picker, because a
// reader can find their own language even when the screen is in one they cannot
// read. It also makes the labels language-independent, which is why they are set
// once here rather than per request — and why TranslateDocType leaves a field
// that carries its own OptionLabels alone.
func applyLanguageOptions(reg *meta.Registry, i18n *I18n) {
	d, ok := reg.Get(languageDocType)
	if !ok {
		return
	}
	f := d.Field(languageFieldname)
	if f == nil || f.Fieldtype != "Select" {
		return
	}
	langs := i18n.Langs()
	// []any, not []string: that is what a Select's options are everywhere else,
	// because they arrive from the TypeScript registry as JSON.
	opts := make([]any, len(langs))
	labels := make([]string, len(langs))
	for i, l := range langs {
		opts[i] = l
		labels[i] = LanguageName(l)
	}
	// fresh slices: a translated copy shares this field's Options by value
	f.Options = opts
	f.OptionLabels = labels
}

// LanguageName is the autonym of a language tag — its name in itself. An
// unparseable tag, or one x/text has no name for, falls back to the code: "eo"
// is more honest to show than a blank entry.
//
// CLDR writes most autonyms in lower case ("português", "français") because
// that is how those languages spell them in running text; English capitalises
// its own. In a picker they are list entries, not running text, so the first
// letter is capitalised to keep the list even.
func LanguageName(code string) string {
	tag, err := language.Parse(code)
	if err != nil {
		return code
	}
	name := display.Self.Name(tag)
	if name == "" {
		return code
	}
	return capitalizeFirst(name, tag)
}

// capitalizeFirst upper-cases the first rune under the language's own casing
// rules — Turkish "i" becomes "İ", not "I".
func capitalizeFirst(s string, tag language.Tag) string {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError {
		return s
	}
	return cases.Upper(tag).String(string(r)) + s[size:]
}
