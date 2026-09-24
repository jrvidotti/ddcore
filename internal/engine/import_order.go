package engine

// The order an import loads DocTypes in (DAT-01).
//
// A Link is checked when the row is written, so whatever it points at has to be
// there already: Users before Projects, Projects before Tasks. That is a
// topological sort over the set being loaded — and where the graph has a cycle,
// including the self link every amendment carries, no order satisfies it. Those
// links are marked deferred: the row goes in without them being checked, and
// the finalize pass verifies them all at once, against the whole loaded set.

import (
	"sort"

	"github.com/jrvidotti/ddcore/internal/meta"
)

// ImportStage is one DocType's place in the order, with the fields whose links
// this order cannot satisfy.
type ImportStage struct {
	Doctype  string
	Deferred []string // fieldnames checked after the load, not at insert
}

// DoctypeLookup resolves a DocType by name, as Ctx.St.DocType does.
type DoctypeLookup func(string) (*meta.DocType, error)

// ImportOrder sorts the DocTypes so that a link's target is loaded before the
// link. Ties are broken by name, so two runs over the same export agree.
func ImportOrder(doctypes []string, lookup DoctypeLookup) ([]ImportStage, error) {
	inSet := map[string]bool{}
	for _, n := range doctypes {
		inSet[n] = true
	}
	deps := map[string]map[string]bool{}
	dyn := map[string][]string{}
	for _, n := range doctypes {
		d, err := lookup(n)
		if err != nil {
			return nil, err
		}
		deps[n] = map[string]bool{}
		for _, f := range linkFields(d, lookup) {
			switch f.Fieldtype {
			case "Dynamic Link":
				dyn[n] = append(dyn[n], f.Fieldname)
			case "Link":
				if t := f.OptionsString(); inSet[t] && t != n {
					deps[n][t] = true
				}
			}
		}
	}

	names := append([]string(nil), doctypes...)
	sort.Strings(names)
	var order []string
	placed := map[string]bool{}
	for len(order) < len(names) {
		var ready []string
		for _, n := range names {
			if placed[n] {
				continue
			}
			free := true
			for t := range deps[n] {
				if !placed[t] {
					free = false
					break
				}
			}
			if free {
				ready = append(ready, n)
			}
		}
		if len(ready) == 0 {
			// Every remaining DocType waits on another: a cycle. Break it at
			// the one waiting on the fewest, so the fewest links are deferred.
			ready = []string{fewestDeps(names, placed, deps)}
		}
		for _, n := range ready {
			order = append(order, n)
			placed[n] = true
		}
	}

	pos := map[string]int{}
	for i, n := range order {
		pos[n] = i
	}
	stages := make([]ImportStage, 0, len(order))
	for _, n := range order {
		d, err := lookup(n)
		if err != nil {
			return nil, err
		}
		var deferred []string
		for _, f := range linkFields(d, lookup) {
			switch f.Fieldtype {
			case "Dynamic Link":
				deferred = append(deferred, f.Fieldname)
			case "Link":
				t := f.OptionsString()
				if inSet[t] && (t == n || pos[t] > pos[n]) {
					deferred = append(deferred, f.Fieldname)
				}
			}
		}
		sort.Strings(deferred)
		stages = append(stages, ImportStage{Doctype: n, Deferred: deferred})
	}
	return stages, nil
}

// fewestDeps picks the DocType to load first inside a cycle: the one with the
// fewest unplaced dependencies, by name where that ties.
func fewestDeps(names []string, placed map[string]bool, deps map[string]map[string]bool) string {
	best, bestN := "", 0
	for _, n := range names {
		if placed[n] {
			continue
		}
		count := 0
		for t := range deps[n] {
			if !placed[t] {
				count++
			}
		}
		if best == "" || count < bestN {
			best, bestN = n, count
		}
	}
	return best
}

// linkFields are a DocType's own Link and Dynamic Link fields, plus those of
// its child tables: a child row is written with its parent, so the parent
// carries the dependency.
func linkFields(d *meta.DocType, lookup DoctypeLookup) []*meta.Field {
	var out []*meta.Field
	for _, f := range d.Fields {
		switch f.Fieldtype {
		case "Link", "Dynamic Link":
			out = append(out, f)
		case "Table", "Table MultiSelect":
			child, err := lookup(f.OptionsString())
			if err != nil || child == nil {
				continue
			}
			for _, cf := range child.Fields {
				if cf.Fieldtype == "Link" || cf.Fieldtype == "Dynamic Link" {
					// The parent's dependency, but not the parent's field: a
					// deferred child link is named by its own fieldname under
					// the table, and the finalize pass walks the child table.
					out = append(out, &meta.Field{Fieldname: f.Fieldname + "." + cf.Fieldname, Fieldtype: cf.Fieldtype, Options: cf.Options})
				}
			}
		}
	}
	return out
}
