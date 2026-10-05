package icons

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the icon list in docs/agent/report-api.md")

func TestTable(t *testing.T) {
	tb := load()
	for name, paths := range tb.Icons {
		if len(paths) == 0 {
			t.Errorf("icon %q has no path", name)
		}
		for _, d := range paths {
			if !strings.HasPrefix(d, "M") && !strings.HasPrefix(d, "m") {
				t.Errorf("icon %q: path %q does not start with a moveto", name, d)
			}
		}
	}
	for alias, name := range tb.Aliases {
		if _, ok := tb.Icons[alias]; ok {
			t.Errorf("alias %q shadows an icon", alias)
		}
		if _, ok := tb.Icons[name]; !ok {
			t.Errorf("alias %q names %q, which is not an icon", alias, name)
		}
	}
	if !Known("wallet") || !Known("triangle-alert") || Known("no-such-icon") || Known("") {
		t.Error("Known answers wrong")
	}
}

// TestNoDuplicateKeys reads icons.json token by token: encoding/json keeps the
// last of two equal keys without a word, so a name added twice would hide one.
func TestNoDuplicateKeys(t *testing.T) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var walk func(path string)
	walk = func(path string) {
		tok, err := dec.Token()
		if err != nil {
			t.Fatal(err)
		}
		switch tok {
		case json.Delim('{'):
			seen := map[string]bool{}
			for dec.More() {
				k, err := dec.Token()
				if err != nil {
					t.Fatal(err)
				}
				key := k.(string)
				if seen[key] {
					t.Errorf("icons.json: %q appears twice", path+key)
				}
				seen[key] = true
				walk(path + key + ".")
			}
			dec.Token()
		case json.Delim('['):
			for dec.More() {
				walk(path)
			}
			dec.Token()
		}
	}
	walk("")
}

const docPath = "../../docs/agent/report-api.md"

// docList is the paragraph report-api.md lists the icons in.
func docList() string {
	names := Names()
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "`" + n + "`"
	}
	aliases := Aliases()
	keys := make([]string, 0, len(aliases))
	for k := range aliases {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, len(keys))
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("`%s` (`%s`)", k, aliases[k])
	}
	return "<!-- icons:begin — generated from desk/icons/icons.json by `go test ./desk/icons -update` -->\n" +
		"Icons: a subset of lucide, the only names the desk draws: " + strings.Join(quoted, ", ") + ".\n" +
		"Other lucide names for some of them work too: " + strings.Join(pairs, ", ") + ".\n" +
		"Any other name in a workspace, a sidebar item, a shortcut or a DocType's `icon` fails the load.\n" +
		"<!-- icons:end -->"
}

// TestDocListsTheTable keeps report-api.md's icon list equal to the table;
// `go test ./desk/icons -update` rewrites it.
func TestDocListsTheTable(t *testing.T) {
	b, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatal(err)
	}
	doc := string(b)
	start := strings.Index(doc, "<!-- icons:begin")
	end := strings.Index(doc, "<!-- icons:end -->")
	if start < 0 || end < start {
		t.Fatalf("%s has no icons:begin … icons:end region", docPath)
	}
	end += len("<!-- icons:end -->")
	want := docList()
	if doc[start:end] == want {
		return
	}
	if !*update {
		t.Fatalf("%s lists other icons than desk/icons/icons.json; run `GOWORK=off go test ./desk/icons -update`", docPath)
	}
	if err := os.WriteFile(docPath, []byte(doc[:start]+want+doc[end:]), 0o644); err != nil {
		t.Fatal(err)
	}
}
