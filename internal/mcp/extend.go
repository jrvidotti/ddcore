package mcp

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/engine"
	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/scaffold"
)

type extendInput struct {
	App     string                 `json:"app" jsonschema:"the extending app (directory), not the DocType's owner"`
	Doctype string                 `json:"doctype" jsonschema:"the DocType to extend, owned by another app or by core"`
	Spec    scaffold.ExtensionSpec `json:"spec" jsonschema:"fields (each may carry insertAfter), set (per-field property overrides), doctype (DocType property overrides), permissions (extra roles), withForm"`
}

// extendDescription lists what an extension may override from the same maps
// ApplyExtensions checks, so the tool cannot promise more than the load allows.
func extendDescription() string {
	keys := func(m map[string]bool) string {
		out := make([]string, 0, len(m))
		for k := range m {
			out = append(out, k)
		}
		sort.Strings(out)
		return strings.Join(out, ", ")
	}
	return "Creates extensions/<snake>.extend.ts in an app: an extendDoctype that adds fields to, and overrides properties of, " +
		"a DocType another app (or core) owns — Frappe's Custom Field and Property Setter, as a versioned file. " +
		"Only creates: an existing extend file is edited directly. The file is loaded before the tool returns; " +
		"if the meta refuses it (fieldname taken, property another app already sets, role already granted) the file is removed and the error returned. " +
		"The host app must be in the extending app's defineApp requires (core is implicit). " +
		"Overridable field properties (set): " + keys(meta.FieldProps) + " — options only on a Select. " +
		"Overridable DocType properties (doctype): " + keys(meta.DoctypeProps) + ". " +
		"Never overridable: fieldname, fieldtype, a Link's or Table's options, idGeneration, isChild, isSingle, isTree, parentField, submittable, module. " +
		"See ddcore://docs/extending."
}

// extendDoctype scaffolds an extension and loads it, leaving nothing behind
// when the load refuses it: unlike a DocType of its own, a clashing extension
// stops the whole site from loading.
func extendDoctype(e *engine.Engine, in extendInput) (map[string]any, error) {
	st := e.Current()
	app := st.App(in.App)
	if app.Name == "" || app.Embedded != nil {
		return nil, cerr.NotFound("app {0} does not exist (apps: {1})", in.App, e.AppOrder())
	}
	d, ok := st.Meta.Get(in.Doctype)
	if !ok {
		return nil, cerr.NotFound("DocType {0} does not exist", in.Doctype)
	}
	if d.App == app.Name {
		return nil, cerr.Validation("app {0} owns {1}: add the field to its .doctype.ts instead", app.Name, d.Name)
	}
	var requires []string
	if am := st.Snap.Apps[app.Name]; am != nil {
		requires = am.Requires
	}
	if d.App != "core" && !slices.Contains(requires, d.App) {
		return nil, cerr.Validation("{0} belongs to app {1}: add {1} to requires in {2} first",
			d.Name, d.App, filepath.Join(app.Dir, "ddcore.app.ts"))
	}

	files, err := scaffold.Extension(app.Dir, d.Name, in.Spec)
	if err != nil {
		return nil, err
	}
	if err := e.Load(); err != nil {
		for _, f := range files {
			os.Remove(f)
		}
		return nil, fmt.Errorf("the extension does not load, so %v were removed: %w", files, err)
	}

	var next []string
	if len(in.Spec.Fields) > 0 {
		next = append(next, "migrate to add the column(s)", "generate_types")
	}
	if extendsText(in.Spec) {
		next = append(next, "i18n_extract then set_translations: the text is a key in "+app.Name+"'s catalogue, not the host's")
	}
	return map[string]any{"files": files, "next": next}, nil
}

// extendsText reports whether the spec writes a catalogue key.
func extendsText(s scaffold.ExtensionSpec) bool {
	text := []string{"label", "description", "options", "idLabel"}
	has := func(m map[string]any) bool {
		for _, k := range text {
			if _, ok := m[k]; ok {
				return true
			}
		}
		return false
	}
	for _, f := range s.Fields {
		if has(f) {
			return true
		}
	}
	for _, props := range s.Set {
		if has(props) {
			return true
		}
	}
	return has(s.Doctype)
}
