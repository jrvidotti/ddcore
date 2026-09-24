package i18nx

import (
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/js"
	"github.com/jrvidotti/ddcore/internal/meta"
)

// CollectDocType collects the human-facing text of one DocType: its own label
// and description, and each field's — layout breaks included, since a Section
// Break's label is a heading on the form.
//
// A Select's options are collected too. The value stays canonical English in
// the database; what is translated is its display label, and the value *is*
// that label's key. Leaving them out would make every status silently
// untranslatable, in the one place a `--check` could not warn about it —
// because at render time the key is computed, not written.
//
// A Link's or a Table's options name a DocType and are an identifier: those
// are never collected.
//
// Only the text `app` owns is collected. A DocType is usually all one app's,
// but a field another app added with extendDoctype — or a label it overrode —
// is written in *that* app's source, and belongs in its catalogue. Attributing
// it to the host would make `--check` demand a translation from the app that
// never wrote the string.
func CollectDocType(s *Set, d *meta.DocType, app, file string) {
	if d.TextAppOf() == app {
		s.Add(d.Label, file, 0)
		s.Add(d.IDLabel, file, 0)
		s.Add(d.Description, file, 0)
	}
	for _, f := range d.Fields {
		if d.FieldTextApp(f) != app {
			continue
		}
		s.Add(f.Label, file, 0)
		s.Add(f.Description, file, 0)
		if f.Fieldtype != "Select" {
			continue
		}
		if f.OptionLabels != nil {
			// self-describing: the field supplies its own display text, so the
			// catalogue is not involved and its options are not keys. See
			// engine.applyLanguageOptions.
			continue
		}
		for _, o := range selectOptions(f) {
			s.Add(o, file, 0)
		}
	}
}

// CollectWorkflow collects a workflow's state and action names: the desk shows
// both through __(), so each name is a key. Its roles are not collected here:
// each app's declared roles are (see Extract).
func CollectWorkflow(s *Set, wf js.Workflow, file string) {
	for _, st := range wf.States {
		s.Add(st.State, file, 0)
	}
	for _, tr := range wf.Transitions {
		s.Add(tr.Action, file, 0)
	}
}

// CollectPortal collects what a portal shows its user: its title, and each
// page's label, description and action labels.
func CollectPortal(s *Set, p engine.Portal, file string) {
	if p.Title != "" {
		s.Add(p.Title, file, 0)
	}
	for _, pg := range p.Pages {
		s.Add(pg.Label, file, 0)
		if pg.Description != "" {
			s.Add(pg.Description, file, 0)
		}
		for _, a := range pg.Actions {
			s.Add(a.Label, file, 0)
		}
	}
}

// CollectPrintTemplate collects a print template's label: the format list
// shows it through c.T. A template without a label shows its name, which is an
// identifier and is not collected.
func CollectPrintTemplate(s *Set, pt engine.PrintTemplate, file string) {
	if pt.Label != "" {
		s.Add(pt.Label, file, 0)
	}
}

func selectOptions(f *meta.Field) []string {
	switch v := f.Options.(type) {
	case []string:
		return v
	case []any:
		out := make([]string, 0, len(v))
		for _, o := range v {
			if str, ok := o.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
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

// CollectReport collects a report's tree and, when it declares no label, its
// name: the runtime shows and translates the name in that case
// (orStr(label, name)), so the name is a catalogue key there too.
func CollectReport(s *Set, rep map[string]any, file string) {
	CollectTree(s, rep, file)
	if l, _ := rep["label"].(string); l != "" {
		return
	}
	if n, _ := rep["name"].(string); n != "" {
		s.Add(n, file, 0)
	}
}
