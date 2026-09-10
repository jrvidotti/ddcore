package i18nx

import (
	"github.com/jrvidotti/ddcore/internal/meta"
)

// CollectDocType collects the human-facing text of one DocType: its own label
// and description, and each field's — layout breaks included, since a Section
// Break's label is a heading on the form.
//
// `options` is never collected: a Select value is canonical English in the
// database, and its display label is the value translated at render time.
func CollectDocType(s *Set, d *meta.DocType, file string) {
	s.Add(d.Label, file, 0)
	s.Add(d.Description, file, 0)
	for _, f := range d.Fields {
		s.Add(f.Label, file, 0)
		s.Add(f.Description, file, 0)
	}
}

// CollectTree collects from the JSON trees of workspaces, reports and apps,
// touching only meta.TreeLabelKeys — the same closed list the runtime
// translates by, so the extractor and the runtime cannot drift apart.
func CollectTree(s *Set, v any, file string) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if str, ok := val.(string); ok {
				if meta.TreeLabelKeys[k] {
					s.Add(str, file, 0)
				}
				continue
			}
			CollectTree(s, val, file)
		}
	case []any:
		for _, val := range t {
			CollectTree(s, val, file)
		}
	}
}
