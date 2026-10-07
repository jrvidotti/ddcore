package api

import (
	"fmt"
	"slices"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// sortWorkspaces puts boot's workspaces in alphabetical order of the label the
// desk shows (the name when there is none), collated for lang so an accented
// or lower-case label sits where a reader of that language looks for it. The
// snapshot keeps workspaces in a map, so without this the switcher, and the
// first workspace the desk lands on, changed from one boot to the next.
func sortWorkspaces(list []map[string]any, lang string) {
	tag, err := language.Parse(lang)
	if err != nil {
		tag = language.English
	}
	col := collate.New(tag, collate.IgnoreCase)
	name := func(ws map[string]any) string { return fmt.Sprint(ws["name"]) }
	shown := func(ws map[string]any) string {
		if l, _ := ws["label"].(string); l != "" {
			return l
		}
		return name(ws)
	}
	slices.SortStableFunc(list, func(a, b map[string]any) int {
		if c := col.CompareString(shown(a), shown(b)); c != 0 {
			return c
		}
		return col.CompareString(name(a), name(b))
	})
}
