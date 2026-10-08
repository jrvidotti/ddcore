package engine

import (
	"fmt"
	"sort"

	"github.com/jrvidotti/ddcore/desk/icons"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// iconError refuses an `icon` the desk does not draw — on a DocType, in a
// Select's optionIcons, on a workspace, or on anything a workspace lists (sidebar items, shortcuts, link
// groups and their items) — because the desk would draw it as a circle and
// nothing else would tell.
func iconError(reg *meta.Registry, workspaces map[string]map[string]any) error {
	bad := func(where string, v any) error {
		name, ok := v.(string)
		if !ok || name == "" || icons.Known(name) {
			return nil
		}
		return fmt.Errorf("%s: icon %q is not one the desk draws; pick one from the list under \"Icons\" in the report-api reference", where, name)
	}
	names := reg.Names()
	sort.Strings(names)
	for _, n := range names {
		d, _ := reg.Get(n)
		if err := bad(fmt.Sprintf("DocType %q", n), d.Icon); err != nil {
			return err
		}
		for _, f := range d.Fields {
			values := mapNames(f.OptionIcons)
			sort.Strings(values)
			for _, v := range values {
				if err := bad(fmt.Sprintf("DocType %q, field %q, optionIcons[%q]", n, f.Fieldname, v), f.OptionIcons[v]); err != nil {
					return err
				}
			}
		}
	}
	// walk finds every "icon" key in a workspace, however deep it sits
	var walk func(where string, v any) error
	walk = func(where string, v any) error {
		switch x := v.(type) {
		case map[string]any:
			if err := bad(where, x["icon"]); err != nil {
				return err
			}
			keys := mapNames(x)
			sort.Strings(keys)
			for _, k := range keys {
				if k == "icon" {
					continue
				}
				if l, ok := x[k].([]any); ok {
					if err := walk(where+", "+k, l); err != nil {
						return err
					}
				}
			}
		case []any:
			for i, it := range x {
				if err := walk(fmt.Sprintf("%s[%d]", where, i), it); err != nil {
					return err
				}
			}
		}
		return nil
	}
	wsNames := mapNames(workspaces)
	sort.Strings(wsNames)
	for _, n := range wsNames {
		if err := walk(fmt.Sprintf("workspace %q", n), workspaces[n]); err != nil {
			return err
		}
	}
	return nil
}
