package meta

import (
	"sort"
	"strings"
)

// SourceDocTypeField is the column every virtual DocType carries: the source
// DocType a row came from. The `_doctype` suffix is what makes the desk show
// it as a DocType select and a DocType label with no code of its own.
const SourceDocTypeField = "source_doctype"

// VirtualSep separates the source DocType from the source id in a virtual
// row's id and in a Link to a virtual DocType: "Person:abc123". A DocType name
// never contains it, so splitting at the first one is unambiguous.
const VirtualSep = ":"

// VirtualDef declares a virtual DocType as the union of local DocTypes.
type VirtualDef struct {
	Sources []VirtualSource `json:"sources"`
}

// VirtualSource is one DocType contributing rows, and which of its fields
// fills each field of the virtual DocType (virtual fieldname -> source
// fieldname). A virtual field the map leaves out reads as NULL on its rows.
type VirtualSource struct {
	DocType string            `json:"doctype"`
	Fields  map[string]string `json:"fields"`
}

// IsVirtual reports whether the DocType is a union of other DocTypes, with no
// table of its own.
func (d *DocType) IsVirtual() bool { return d.Virtual != nil }

// VirtualSource returns the source declaration for a DocType, or nil.
func (d *DocType) VirtualSource(doctype string) *VirtualSource {
	if d.Virtual == nil {
		return nil
	}
	for i := range d.Virtual.Sources {
		if d.Virtual.Sources[i].DocType == doctype {
			return &d.Virtual.Sources[i]
		}
	}
	return nil
}

// VirtualID is the id of a source row inside a virtual DocType.
func VirtualID(source, id string) string { return source + VirtualSep + id }

// SplitVirtualID splits a virtual id into its source DocType and source id.
func SplitVirtualID(v string) (source, id string, ok bool) {
	source, id, ok = strings.Cut(v, VirtualSep)
	if !ok || source == "" || id == "" {
		return "", "", false
	}
	return source, id, true
}

// VirtualsOf lists the virtual DocTypes that take rows from doctype, by name.
func (r *Registry) VirtualsOf(doctype string) []*DocType {
	var out []*DocType
	for _, d := range r.DocTypes {
		if d.VirtualSource(doctype) != nil {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ApplyVirtual adds the `source_doctype` field to every virtual DocType that
// does not declare it. Declaring it yourself is how you place, relabel or hide
// it; validateVirtual refuses a declaration that is not plain Data.
func (r *Registry) ApplyVirtual() {
	for _, d := range r.DocTypes {
		if !d.IsVirtual() || d.Field(SourceDocTypeField) != nil {
			continue
		}
		d.Fields = append(d.Fields, &Field{
			Fieldname:        SourceDocTypeField,
			Fieldtype:        "Data",
			Label:            "Source",
			InListView:       true,
			InStandardFilter: true,
			ReadOnly:         true,
		})
		d.ResetFieldIndex()
	}
}

// virtualPerms are the rights a virtual DocType may grant: everything that
// reads, nothing that writes.
func virtualPermWrites(p Perm) bool {
	return p.Write || p.Create || p.Delete || p.Submit || p.Cancel || p.Amend || p.Import || p.Share
}

// validateVirtual checks a union's declaration against the registry. It is
// all refused at load: a mapping that names a missing column or a column of
// another type would not be a wrong row, it would be a query that cannot run.
func validateVirtual(r *Registry, d *DocType, e func(string, ...any)) {
	if !d.IsVirtual() {
		return
	}
	if d.IsChild || d.IsSingle || d.IsTree || d.Submittable {
		e("a virtual DocType cannot be a child table, a Single, a tree, or submittable")
	}
	if d.TrackChanges || d.AllowRename {
		e("a virtual DocType has no writes, so no trackChanges or allowRename")
	}
	if d.IDGeneration != (IDGeneration{}) {
		e("a virtual DocType cannot declare idGeneration: its ids are <source>:<id>")
	}
	if len(d.RenamedFrom) > 0 {
		e("a virtual DocType has no table to rename, so no renamedFrom")
	}
	for _, p := range d.Permissions {
		if virtualPermWrites(p) {
			e("permissions for %q: a virtual DocType grants only read, select, report and export", p.Role)
		}
		if p.Permlevel > 0 {
			e("permissions for %q: a virtual DocType has no field levels of its own; its sources' apply", p.Role)
		}
	}
	for _, f := range d.Fields {
		if LayoutTypes[f.Fieldtype] {
			continue
		}
		switch {
		case IsTableType(f.Fieldtype):
			e("field %q: a virtual DocType has no child tables", f.Fieldname)
		case f.Fieldtype == "Dynamic Link":
			e("field %q: a virtual DocType cannot hold a Dynamic Link", f.Fieldname)
		case f.Fieldtype == "Password" || f.Fieldtype == "Vault":
			e("field %q: a virtual DocType cannot expose a %s", f.Fieldname, f.Fieldtype)
		}
		if f.Permlevel > 0 {
			e("field %q: a virtual DocType has no field levels of its own; its sources' apply", f.Fieldname)
		}
		if f.Unique || f.FetchFrom != "" || len(f.RenamedFrom) > 0 || f.Convert != nil {
			e("field %q: unique, fetchFrom, renamedFrom and convert need a table", f.Fieldname)
		}
	}
	if f := d.Field(SourceDocTypeField); f != nil && f.Fieldtype != "Data" {
		e("%s is a %s; it must be Data", SourceDocTypeField, f.Fieldtype)
	}
	if len(d.Virtual.Sources) == 0 {
		e("virtual needs at least one source")
	}
	seen := map[string]bool{}
	for _, s := range d.Virtual.Sources {
		src := r.DocTypes[s.DocType]
		switch {
		case s.DocType == "":
			e("virtual source without a doctype")
			continue
		case seen[s.DocType]:
			e("virtual source %q is declared twice", s.DocType)
			continue
		case src == nil:
			e("virtual source %q does not exist", s.DocType)
			continue
		case strings.Contains(s.DocType, VirtualSep):
			e("virtual source %q: a source's name cannot contain %q, which separates it from the id", s.DocType, VirtualSep)
		case src.IsVirtual() || src.IsChild || src.IsSingle:
			e("virtual source %q is a virtual DocType, a child table or a Single", s.DocType)
			continue
		}
		seen[s.DocType] = true
		for _, vf := range sortedKeys(s.Fields) {
			sf := s.Fields[vf]
			target := d.Field(vf)
			switch {
			case vf == SourceDocTypeField || d.IsStdColumn(vf):
				e("virtual source %q maps %q, which the union fills itself", s.DocType, vf)
				continue
			case target == nil || LayoutTypes[target.Fieldtype]:
				e("virtual source %q maps %q, which is not a field of %s", s.DocType, vf, d.Name)
				continue
			}
			from := src.Field(sf)
			switch {
			case from == nil && src.IsStdColumn(sf):
				// owner, creation… are readable columns of every row; only the
				// ones typed like the target may fill it.
				if ColumnType(target.Fieldtype) != stdColumnType(sf) {
					e("virtual source %q maps %q to %s, a %s column; %q is %s", s.DocType, vf, sf, stdColumnType(sf), vf, ColumnType(target.Fieldtype))
				}
			case from == nil:
				e("virtual source %q maps %q to %q, which is not a field of %s", s.DocType, vf, sf, s.DocType)
			case ColumnType(from.Fieldtype) == "" || from.Fieldtype == "Password":
				e("virtual source %q maps %q to %q, a %s, which has no readable column", s.DocType, vf, sf, from.Fieldtype)
			case ColumnType(from.Fieldtype) != ColumnType(target.Fieldtype):
				e("virtual source %q maps %q (%s) to %q (%s); the column types differ", s.DocType, vf, target.Fieldtype, sf, from.Fieldtype)
			case target.Fieldtype == "Link" && (from.Fieldtype != "Link" || from.OptionsString() != target.OptionsString()):
				e("virtual source %q maps Link %q (to %s) to %q, which is not a Link to %s", s.DocType, vf, target.OptionsString(), sf, target.OptionsString())
			}
		}
	}
}

// stdColumnType is the column type of a standard column, for mapping one.
func stdColumnType(c string) string {
	switch c {
	case "creation", "modified":
		return "timestamptz"
	case "docstatus":
		return "bigint"
	}
	return "text"
}
