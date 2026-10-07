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

// InSpace reports whether a workspace or one of its items shows in space. An
// item that names no space takes the one of what it opens: a DocType that
// lives inside a tenant (#105), or a report on one, is a tenant's item. An
// explicit `space` wins either way.
func (s *State) InSpace(item map[string]any, space string) bool {
	if space == "" {
		return true
	}
	want, _ := item["space"].(string)
	if want == "" && s.opensTenantOnly(item) {
		want = SpaceTenant
	}
	return want == "" || want == space
}

// opensTenantOnly reports whether an item opens a tenant-only DocType: its
// `doctype`, a card's or chart's `refDoctype`, or the `refDoctype` of its
// `report`.
func (s *State) opensTenantOnly(item map[string]any) bool {
	if s == nil || s.Meta == nil {
		return false
	}
	tenantOnly := func(v any) bool {
		name, _ := v.(string)
		d, ok := s.Meta.Get(name)
		return name != "" && ok && d.TenantOnly
	}
	if tenantOnly(item["doctype"]) || tenantOnly(item["refDoctype"]) {
		return true
	}
	if rep, _ := item["report"].(string); rep != "" && s.Snap != nil {
		return tenantOnly(s.Snap.Reports[rep]["refDoctype"])
	}
	return false
}

// WorkspaceForSpace is the workspace as space sees it: nil when the workspace
// belongs to the other space, otherwise a copy without the items that do.
// The grouped links follow their DocType or report alone. The snapshot's own
// map is never changed.
func (s *State) WorkspaceForSpace(ws map[string]any, space string) map[string]any {
	if !s.InSpace(ws, space) {
		return nil
	}
	if space == "" {
		return ws
	}
	out := make(map[string]any, len(ws))
	for k, v := range ws {
		out[k] = v
	}
	keep := func(list []any) []any {
		kept := make([]any, 0, len(list))
		for _, it := range list {
			if m, ok := it.(map[string]any); !ok || s.InSpace(m, space) {
				kept = append(kept, it)
			}
		}
		return kept
	}
	for _, key := range workspaceSpaceLists {
		if list, ok := ws[key].([]any); ok {
			out[key] = keep(list)
		}
	}
	if groups, ok := ws["links"].([]any); ok {
		kept := make([]any, 0, len(groups))
		for _, g := range groups {
			gm, ok := g.(map[string]any)
			items, _ := gm["items"].([]any)
			if !ok || items == nil {
				kept = append(kept, g)
				continue
			}
			left := keep(items)
			if len(left) == 0 && len(items) > 0 {
				continue // a heading over nothing
			}
			copied := make(map[string]any, len(gm))
			for k, v := range gm {
				copied[k] = v
			}
			copied["items"] = left
			kept = append(kept, copied)
		}
		out["links"] = kept
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
