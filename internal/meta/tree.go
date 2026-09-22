package meta

import "strings"

// IsGroupField is the Check every tree DocType carries: only a document with it
// set may have children. A leaf is a document, a group is a folder, and the
// distinction is what keeps a hierarchy from growing sideways under a record.
const IsGroupField = "is_group"

// TreeParentField is the Link field holding a document's parent, or "" when the
// DocType is not a tree. The default follows Frappe's convention,
// `parent_<snake(name)>`, so a declaration only names it to say otherwise.
func (d *DocType) TreeParentField() string {
	if !d.IsTree {
		return ""
	}
	if d.ParentField != "" {
		return d.ParentField
	}
	return "parent_" + Snake(d.Name)
}

// ApplyTrees adds the parent Link and the `is_group` Check to every tree
// DocType that does not declare them. Declaring either yourself is how you
// place it in the layout, relabel it or carry a `renamedFrom`; what a
// declaration may *not* do — point the Link elsewhere, make it mandatory, raise
// its permission level — is refused by validateTree.
//
// It runs before ApplyExtensions, so an extension can override the injected
// fields' label or description like any other field's.
func (r *Registry) ApplyTrees() {
	for _, d := range r.DocTypes {
		if !d.IsTree {
			continue
		}
		pf := d.TreeParentField()
		// The default is written back, so everything downstream — the desk's
		// meta, the typings, a controller — reads the parent field by name
		// instead of having to derive it.
		d.ParentField = pf
		added := false
		if pf != "" && d.Field(pf) == nil {
			label := d.Label
			if label == "" {
				label = d.Name
			}
			d.Fields = append(d.Fields, &Field{
				Fieldname:        pf,
				Fieldtype:        "Link",
				Label:            "Parent " + label,
				Options:          d.Name,
				InStandardFilter: true,
			})
			added = true
		}
		if d.Field(IsGroupField) == nil {
			d.Fields = append(d.Fields, &Field{
				Fieldname:  IsGroupField,
				Fieldtype:  "Check",
				Label:      "Is Group",
				InListView: true,
			})
			added = true
		}
		if added {
			d.ResetFieldIndex()
		}
	}
}

// validateTree checks a hierarchy's two fields. Everything here is refused at
// load: the recursive queries read both columns on every scoped list, so a
// parent that points elsewhere or an is_group behind a permission level would
// not be a wrong answer, it would be no answer at all.
func validateTree(d *DocType, e func(string, ...any)) {
	if !d.IsTree {
		if d.ParentField != "" {
			e("parentField needs isTree")
		}
		return
	}
	if d.IsChild || d.IsSingle || d.Submittable {
		e("a tree DocType cannot be a child table, a Single, or submittable")
	}
	pf := d.TreeParentField()
	switch {
	case !fieldnameRe.MatchString(pf):
		e("parentField %q is not a fieldname (use ascii snake_case)", pf)
	case d.IsStdColumn(pf) || pf == "doctype":
		e("parentField %q is a reserved column", pf)
	case pf == "parent" || pf == "parenttype" || pf == "parentfield":
		// `parent` is the column a child row uses for *its* parent document,
		// and the child EXISTS in a list query joins on it.
		e("parentField %q is the child-row column; call it parent_<something>", pf)
	case pf == IsGroupField:
		e("parentField cannot be %q", IsGroupField)
	}
	if f := d.Field(pf); f != nil {
		switch {
		case f.Fieldtype != "Link":
			e("parentField %q is a %s; it must be a Link", pf, f.Fieldtype)
		case !strings.EqualFold(f.OptionsString(), d.Name):
			e("parentField %q points at %q; a tree's parent points at its own DocType", pf, f.OptionsString())
		}
		if f.Reqd {
			e("parentField %q cannot be reqd: a root has no parent", pf)
		}
		if f.Unique {
			e("parentField %q cannot be unique: siblings share a parent", pf)
		}
		if f.Permlevel > 0 {
			e("parentField %q cannot sit above permission level 0: the tree queries read it", pf)
		}
		if f.FetchFrom != "" {
			e("parentField %q cannot be fetched from another field", pf)
		}
	}
	if f := d.Field(IsGroupField); f != nil {
		if f.Fieldtype != "Check" {
			e("%s is a %s; it must be a Check", IsGroupField, f.Fieldtype)
		}
		if f.Permlevel > 0 {
			e("%s cannot sit above permission level 0: the tree queries read it", IsGroupField)
		}
	}
}
