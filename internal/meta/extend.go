package meta

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// Extension is one `extendDoctype` call: what an app adds to, or changes on, a
// DocType another app owns.
//
// The merge happens once, here, before the registry is validated — so a field
// an extension adds is a field like any other by the time DDL, typegen, the
// API and the desk see it. Nothing downstream needs to know it came from
// somewhere else.
type Extension struct {
	Doctype     string                    `json:"doctype"`
	App         string                    `json:"app"`
	SourceFile  string                    `json:"sourceFile"`
	Fields      []*ExtField               `json:"fields,omitempty"`
	Set         map[string]map[string]any `json:"set,omitempty"`
	Doc         map[string]any            `json:"props,omitempty"`
	Permissions []Perm                    `json:"permissions,omitempty"`
}

// ExtField is a field an extension adds, plus where to put it.
type ExtField struct {
	Field
	InsertAfter string `json:"insertAfter,omitempty"`
}

// Apps is the app graph the merge needs: the load order (which decides the
// order extensions apply in, and so the order of the fields they add) and what
// each app declares it requires.
type Apps struct {
	Order    []string
	Requires map[string][]string
}

// FieldProps are the field properties an extension may override. Deliberately
// a closed list: `fieldname` and `fieldtype` are the column's identity, and a
// Link's or a Table's `options` is what it points at — changing either from
// another app turns a property setter into a silent schema change.
var FieldProps = map[string]bool{
	"label": true, "description": true, "reqd": true, "unique": true, "default": true,
	"readOnly": true, "hidden": true, "dependsOn": true, "readOnlyDependsOn": true,
	"mandatoryDependsOn": true, "allowOnSubmit": true, "inListView": true,
	"inStandardFilter": true, "searchIndex": true, "length": true, "precision": true,
	"columns": true, "width": true, "gridEditMode": true, "collapsible": true, "bold": true,
	"optionColors": true, "options": true, "permlevel": true,
}

// DoctypeProps are the DocType properties an extension may override. `naming`,
// `isChild`, `isSingle` and `submittable` decide what the document *is*, and
// stay with the app that declares it.
var DoctypeProps = map[string]bool{
	"label": true, "description": true, "icon": true, "titleField": true,
	"sortField": true, "sortOrder": true, "searchFields": true,
	"trackChanges": true, "allowRename": true,
}

// textProps are the properties whose value is a catalogue key. Overriding one
// moves the key into the extending app's CSV — see Field.App.
var textProps = map[string]bool{"label": true, "description": true, "options": true}

// ApplyExtensions merges every extension into the registry.
//
// It reports every problem it finds rather than the first: a migration that
// ports a dozen custom fields at once should see the whole list. Two apps
// changing the same property is one of those problems — the effective meta
// must not depend on the order the apps happen to be installed in, so the
// conflict refuses to load instead of resolving itself.
func (r *Registry) ApplyExtensions(exts []*Extension, apps Apps) error {
	rank := map[string]int{}
	for i, a := range apps.Order {
		rank[a] = i
	}
	sorted := append([]*Extension(nil), exts...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if a, b := rank[sorted[i].App], rank[sorted[j].App]; a != b {
			return a < b
		}
		return sorted[i].SourceFile < sorted[j].SourceFile
	})

	var errs []string
	// who set what: "<doctype>\x00<fieldname>\x00<prop>", the fieldname empty
	// for a DocType-level property and the prop empty for a permission's role
	claims := map[string]*Extension{}
	for _, e := range sorted {
		fail := func(format string, a ...any) {
			errs = append(errs, fmt.Sprintf("%s: app %q (%s) ", e.Doctype, e.App, e.SourceFile)+fmt.Sprintf(format, a...))
		}

		d, ok := r.Get(e.Doctype)
		if !ok {
			fail("extends a DocType that does not exist")
			continue
		}
		if d.App == e.App {
			fail("extends a DocType it owns — declare the field in the DocType instead")
			continue
		}
		// core is always loaded first, so requiring it says nothing; any other
		// host has to be declared, or the extension could load before it.
		if d.App != "core" && !contains(apps.Requires[e.App], d.App) {
			fail("must declare requires: [%q] in defineApp before extending %s", d.App, d.Name)
			continue
		}

		for _, ef := range e.Fields {
			f := ef.Field
			if f.Fieldname != "" {
				if old := d.Field(f.Fieldname); old != nil {
					fail("adds field %q, which %s already defines", f.Fieldname, ownerOf(d, old))
					continue
				}
			}
			f.App = e.App
			if err := insertField(d, &f, ef.InsertAfter); err != nil {
				fail("%s", err)
				continue
			}
		}

		for _, name := range sortedKeys(e.Set) {
			f := d.Field(name)
			if f == nil {
				fail("sets a property on field %q, which does not exist", name)
				continue
			}
			props := e.Set[name]
			allowed := map[string]any{}
			for _, prop := range sortedKeys(props) {
				if !FieldProps[prop] {
					fail("may not override %q on field %q", prop, name)
					continue
				}
				if prop == "options" && f.Fieldtype != "Select" {
					fail("may not override the options of %q: on a %s they name the target", name, f.Fieldtype)
					continue
				}
				if prev := claims[key(e.Doctype, name, prop)]; prev != nil {
					fail("also sets %q on field %q, already set by app %q (%s)", prop, name, prev.App, prev.SourceFile)
					continue
				}
				claims[key(e.Doctype, name, prop)] = e
				allowed[prop] = props[prop]
				if textProps[prop] {
					f.App = e.App
				}
			}
			if err := overlay(f, allowed); err != nil {
				fail("field %q: %s", name, err)
			}
		}

		for _, prop := range sortedKeys(e.Doc) {
			if !DoctypeProps[prop] {
				fail("may not override %q on the DocType", prop)
				continue
			}
			if prev := claims[key(e.Doctype, "", prop)]; prev != nil {
				fail("also sets %q on the DocType, already set by app %q (%s)", prop, prev.App, prev.SourceFile)
				continue
			}
			claims[key(e.Doctype, "", prop)] = e
			if err := setDoctypeProp(d, prop, e.Doc[prop]); err != nil {
				fail("%s", err)
				continue
			}
			if textProps[prop] {
				d.TextApp = e.App
			}
		}

		for _, p := range e.Permissions {
			if d.IsChild {
				fail("grants permissions on a child DocType, which has none of its own")
				break
			}
			claim := key(e.Doctype, p.Role, fmt.Sprint(p.Permlevel))
			grant := fmt.Sprintf("%q", p.Role)
			if p.Permlevel > 0 {
				grant = fmt.Sprintf("%q at permlevel %d", p.Role, p.Permlevel)
			}
			if prev := claims[claim]; prev != nil {
				fail("also grants %s, already granted by app %q (%s)", grant, prev.App, prev.SourceFile)
				continue
			}
			if hasRole(d.Permissions, p.Role, p.Permlevel) {
				fail("grants %s, which app %q already grants — an extension only adds roles", grant, d.App)
				continue
			}
			claims[claim] = e
			d.Permissions = append(d.Permissions, p)
		}

		if !contains(d.ExtendedBy, e.App) {
			d.ExtendedBy = append(d.ExtendedBy, e.App)
		}
	}

	if len(errs) > 0 {
		sort.Strings(errs)
		return fmt.Errorf("invalid extension:\n  %s", strings.Join(errs, "\n  "))
	}
	return nil
}

// insertField puts f after `after`, or at the end when it is empty.
func insertField(d *DocType, f *Field, after string) error {
	defer d.ResetFieldIndex()
	if after == "" {
		d.Fields = append(d.Fields, f)
		return nil
	}
	for i, x := range d.Fields {
		if x.Fieldname == after {
			d.Fields = append(d.Fields[:i+1], append([]*Field{f}, d.Fields[i+1:]...)...)
			return nil
		}
	}
	return fmt.Errorf("inserts field %q after %q, which does not exist", f.Fieldname, after)
}

// overlay writes the properties onto the field through its JSON tags, so a
// value declared in TypeScript lands with the same rules the meta arrived by.
func overlay(f *Field, props map[string]any) error {
	if len(props) == 0 {
		return nil
	}
	cur, err := json.Marshal(f)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(cur, &m); err != nil {
		return err
	}
	for k, v := range props {
		m[k] = v
	}
	b, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, f)
}

func setDoctypeProp(d *DocType, prop string, v any) error {
	str := func() (string, error) {
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("%q must be a string", prop)
		}
		return s, nil
	}
	boolean := func() (bool, error) {
		b, ok := v.(bool)
		if !ok {
			return false, fmt.Errorf("%q must be a boolean", prop)
		}
		return b, nil
	}
	var err error
	switch prop {
	case "label":
		d.Label, err = str()
	case "description":
		d.Description, err = str()
	case "icon":
		d.Icon, err = str()
	case "titleField":
		d.TitleField, err = str()
	case "sortField":
		d.SortField, err = str()
	case "sortOrder":
		d.SortOrder, err = str()
	case "trackChanges":
		d.TrackChanges, err = boolean()
	case "allowRename":
		d.AllowRename, err = boolean()
	case "searchFields":
		list, ok := v.([]any)
		if !ok {
			return fmt.Errorf("%q must be a list", prop)
		}
		out := make([]string, 0, len(list))
		for _, x := range list {
			s, ok := x.(string)
			if !ok {
				return fmt.Errorf("%q must be a list of fieldnames", prop)
			}
			out = append(out, s)
		}
		d.SearchFields = out
	}
	return err
}

// TextAppOf is the app whose catalogue owns this DocType's own text.
func (d *DocType) TextAppOf() string {
	if d.TextApp != "" {
		return d.TextApp
	}
	return d.App
}

// FieldTextApp is the app whose catalogue owns this field's text.
func (d *DocType) FieldTextApp(f *Field) string {
	if f.App != "" {
		return f.App
	}
	return d.App
}

// ownerOf names the app a field's text belongs to, for an error message.
func ownerOf(d *DocType, f *Field) string {
	if f.App != "" && f.App != d.App {
		return fmt.Sprintf("app %q", f.App)
	}
	return fmt.Sprintf("app %q", d.App)
}

func key(doctype, field, prop string) string { return doctype + "\x00" + field + "\x00" + prop }

func hasRole(perms []Perm, role string, permlevel int) bool {
	for _, p := range perms {
		if p.Role == role && p.Permlevel == permlevel {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
