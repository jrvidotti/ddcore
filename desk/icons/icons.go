// Package icons is the desk's icon set: the SVG paths of the lucide icons the
// desk draws, and the names an app may give as the `icon` of a workspace, a
// sidebar item, a shortcut or a DocType. icons.json is the one table: the desk
// imports it to draw, and the engine reads it to refuse a name the desk would
// draw as a circle.
package icons

import (
	_ "embed"
	"encoding/json"
	"sort"
	"sync"
)

//go:embed icons.json
var raw []byte

type table struct {
	// Icons maps a name to the d attribute of each of its paths.
	Icons map[string][]string `json:"icons"`
	// Aliases maps a lucide name to the one the table draws it under, so
	// lucide's renamed forms (triangle-alert for alert-triangle) work too.
	Aliases map[string]string `json:"aliases"`
}

var load = sync.OnceValue(func() table {
	var t table
	if err := json.Unmarshal(raw, &t); err != nil {
		panic("desk/icons: icons.json: " + err.Error())
	}
	return t
})

// Known reports whether the desk draws name, as an icon or an alias.
func Known(name string) bool {
	t := load()
	if _, ok := t.Icons[name]; ok {
		return true
	}
	_, ok := t.Aliases[name]
	return ok
}

// Names are the icons the desk draws, sorted, aliases left out.
func Names() []string {
	t := load()
	out := make([]string, 0, len(t.Icons))
	for n := range t.Icons {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// Aliases maps each alias to the icon it draws.
func Aliases() map[string]string {
	t := load()
	out := make(map[string]string, len(t.Aliases))
	for k, v := range t.Aliases {
		out[k] = v
	}
	return out
}
