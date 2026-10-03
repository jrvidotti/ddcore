package engine

import (
	"fmt"
	"sort"
)

// A workspace, and each of its sidebar items, shortcuts and number cards, may
// name the space it belongs to on a site with tenancy: "platform" or
// "tenant". Without one it shows in both; without tenancy the field means
// nothing.
const (
	SpacePlatform = "platform"
	SpaceTenant   = "tenant"
)

// workspaceSpaceLists are the lists of a workspace whose items carry a space.
var workspaceSpaceLists = []string{"sidebar", "shortcuts", "numberCards"}

// Space is the space this ctx works in, as a workspace names it: "tenant"
// inside a tenant, "platform" outside one, and "" on a site without tenancy,
// where every item shows.
func (c *Ctx) Space() string {
	if !c.Tenancy() {
		return ""
	}
	if c.Tenant != "" {
		return SpaceTenant
	}
	return SpacePlatform
}

// InSpace reports whether a workspace or one of its items shows in space.
func InSpace(item map[string]any, space string) bool {
	want, _ := item["space"].(string)
	return space == "" || want == "" || want == space
}

// WorkspaceForSpace is the workspace as space sees it: nil when the workspace
// belongs to the other space, otherwise a copy without the items that do.
// The snapshot's own map is never changed.
func WorkspaceForSpace(ws map[string]any, space string) map[string]any {
	if !InSpace(ws, space) {
		return nil
	}
	if space == "" {
		return ws
	}
	out := make(map[string]any, len(ws))
	for k, v := range ws {
		out[k] = v
	}
	for _, key := range workspaceSpaceLists {
		list, ok := ws[key].([]any)
		if !ok {
			continue
		}
		kept := make([]any, 0, len(list))
		for _, it := range list {
			if m, ok := it.(map[string]any); !ok || InSpace(m, space) {
				kept = append(kept, it)
			}
		}
		out[key] = kept
	}
	return out
}

// workspaceSpaceError refuses a space other than "platform" or "tenant", on a
// workspace or on one of its items, so a typo does not hide an item silently.
func workspaceSpaceError(workspaces map[string]map[string]any) error {
	check := func(where string, item map[string]any) error {
		v, ok := item["space"]
		if !ok || v == nil || v == SpacePlatform || v == SpaceTenant {
			return nil
		}
		return fmt.Errorf("%s: space %v is neither %q nor %q", where, v, SpacePlatform, SpaceTenant)
	}
	names := mapNames(workspaces)
	sort.Strings(names)
	for _, name := range names {
		ws := workspaces[name]
		if err := check(fmt.Sprintf("workspace %q", name), ws); err != nil {
			return err
		}
		for _, key := range workspaceSpaceLists {
			list, _ := ws[key].([]any)
			for i, it := range list {
				m, _ := it.(map[string]any)
				if m == nil {
					continue
				}
				if err := check(fmt.Sprintf("workspace %q, %s[%d]", name, key, i), m); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
