package meta

import "testing"

func treeReg(t *testing.T, d *DocType) *Registry {
	t.Helper()
	r := NewRegistry()
	if err := r.Add(d); err != nil {
		t.Fatalf("add: %v", err)
	}
	r.ApplyTrees()
	return r
}

func TestApplyTreesAddsFields(t *testing.T) {
	d := &DocType{Name: "Task Category", Label: "Task Category", IsTree: true,
		Fields: []*Field{{Fieldname: "title", Fieldtype: "Data"}}}
	r := treeReg(t, d)
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	pf := d.TreeParentField()
	if pf != "parent_task_category" {
		t.Fatalf("parent field = %q", pf)
	}
	f := d.Field(pf)
	if f == nil || f.Fieldtype != "Link" || f.OptionsString() != "Task Category" {
		t.Fatalf("parent field not injected as a self Link: %+v", f)
	}
	if g := d.Field(IsGroupField); g == nil || g.Fieldtype != "Check" {
		t.Fatalf("is_group not injected: %+v", g)
	}
}

func TestApplyTreesKeepsDeclaredFields(t *testing.T) {
	d := &DocType{Name: "Territory", IsTree: true, ParentField: "parent_territory", Fields: []*Field{
		{Fieldname: "parent_territory", Fieldtype: "Link", Label: "Region", Options: "Territory"},
		{Fieldname: IsGroupField, Fieldtype: "Check", Label: "Group"},
	}}
	r := treeReg(t, d)
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if n := len(d.Fields); n != 2 {
		t.Fatalf("declared fields were duplicated: %d fields", n)
	}
	if d.Field("parent_territory").Label != "Region" {
		t.Fatalf("declared label overwritten")
	}
}

func TestApplyTreesCustomParentField(t *testing.T) {
	d := &DocType{Name: "Account", IsTree: true, ParentField: "parent_account_id"}
	r := treeReg(t, d)
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if d.Field("parent_account_id") == nil || d.Field("parent_account") != nil {
		t.Fatalf("custom parentField ignored")
	}
}

func TestTreeUniqueKeysAllowed(t *testing.T) {
	d := &DocType{Name: "Territory", IsTree: true,
		Fields:     []*Field{{Fieldname: "title", Fieldtype: "Data"}},
		UniqueKeys: []UniqueKey{{Name: "sibling_title", Fields: []string{"parent_territory", "title"}}}}
	r := treeReg(t, d)
	if err := r.Validate(); err != nil {
		t.Fatalf("a unique key over the parent and a field is legitimate: %v", err)
	}
}

func TestTreeInvalidMetadata(t *testing.T) {
	cases := map[string]*DocType{
		"child":              {Name: "Line", IsTree: true, IsChild: true},
		"single":             {Name: "Settings", IsTree: true, IsSingle: true},
		"submittable":        {Name: "Entry", IsTree: true, Submittable: true},
		"parentField alone":  {Name: "Flat", ParentField: "parent_flat"},
		"parent column":      {Name: "Node", IsTree: true, ParentField: "parent"},
		"is_group as parent": {Name: "Node", IsTree: true, ParentField: IsGroupField},
		"std column":         {Name: "Node", IsTree: true, ParentField: "owner"},
		"not snake case":     {Name: "Node", IsTree: true, ParentField: "parentNode"},
		"not a link": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "parent_node", Fieldtype: "Data"}}},
		"points elsewhere": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "parent_node", Fieldtype: "Link", Options: "User"}}},
		"mandatory parent": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "parent_node", Fieldtype: "Link", Options: "Node", Reqd: true}}},
		"unique parent": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "parent_node", Fieldtype: "Link", Options: "Node", Unique: true}}},
		"restricted parent": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "parent_node", Fieldtype: "Link", Options: "Node", Permlevel: 1}}},
		"fetched parent": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: "other", Fieldtype: "Link", Options: "Node"},
			{Fieldname: "parent_node", Fieldtype: "Link", Options: "Node", FetchFrom: "other.title"}}},
		"is_group not a check": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: IsGroupField, Fieldtype: "Data"}}},
		"restricted is_group": {Name: "Node", IsTree: true, Fields: []*Field{
			{Fieldname: IsGroupField, Fieldtype: "Check", Permlevel: 2}}},
	}
	for label, d := range cases {
		t.Run(label, func(t *testing.T) {
			r := NewRegistry()
			if err := r.Add(d); err != nil {
				t.Fatalf("add: %v", err)
			}
			r.ApplyTrees()
			if err := r.Validate(); err == nil {
				t.Fatalf("accepted %s", label)
			}
		})
	}
}

func TestTreeParentFieldEmptyWithoutTree(t *testing.T) {
	d := &DocType{Name: "Project"}
	if d.TreeParentField() != "" {
		t.Fatalf("a DocType that is not a tree has no parent field")
	}
}
