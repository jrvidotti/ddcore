package engine

import (
	"fmt"
	"sort"
	"strings"
)

// routeName is the form a DocType, workspace or report name takes in a Desk
// path: the name without its whitespace ("Training Class" → "TrainingClass").
// The desk derives the same form in desk/src/lib/routes.ts.
func routeName(name string) string {
	return strings.Join(strings.Fields(name), "")
}

// routeNameClash refuses two names of one kind that share a route name: the
// Desk would give them the same address and open only one of them.
func routeNameClash(kind string, names []string) error {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	seen := make(map[string]string, len(sorted))
	for _, name := range sorted {
		rn := routeName(name)
		if other, ok := seen[rn]; ok {
			return fmt.Errorf("%s %q: its URL name %q is taken by %s %q", kind, name, rn, kind, other)
		}
		seen[rn] = name
	}
	return nil
}

func mapNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
