package meta

import (
	"strings"
	"testing"
)

func TestSEC02_FieldPermissionValidation(t *testing.T) {
	base := func() (*Registry, *DocType) {
		r := NewRegistry()
		src := &DocType{Name: "Source", Fields: []*Field{
			{Fieldname: "title", Fieldtype: "Data"},
			{Fieldname: "cost", Fieldtype: "Currency", Permlevel: 1},
		}, Permissions: []Perm{{Role: "Staff", Read: true}}}
		d := &DocType{Name: "Target", Fields: []*Field{
			{Fieldname: "title", Fieldtype: "Data"},
			{Fieldname: "source", Fieldtype: "Link", Options: "Source"},
			{Fieldname: "salary", Fieldtype: "Currency", Permlevel: 1},
		}, Permissions: []Perm{
			{Role: "Staff", Read: true, Write: true, Create: true},
			{Role: "HR", Read: true, Write: true, Permlevel: 1},
		}}
		r.Add(src)
		r.Add(d)
		return r, d
	}
	if r, _ := base(); r.Validate() != nil {
		t.Fatalf("valid permlevel meta refused: %v", r.Validate())
	}
	cases := []struct {
		name   string
		mutate func(d *DocType)
		want   string
	}{
		{"field level out of range", func(d *DocType) { d.Fields[2].Permlevel = 10 }, "permlevel 10 is out of range"},
		{"negative field level", func(d *DocType) { d.Fields[2].Permlevel = -1 }, "out of range"},
		{"layout field", func(d *DocType) {
			d.Fields = append(d.Fields, &Field{Fieldname: "sec", Fieldtype: "Section Break", Permlevel: 1})
		}, "holds no value"},
		{"row level out of range", func(d *DocType) { d.Permissions[1].Permlevel = 12 }, "out of range"},
		{"row grants create", func(d *DocType) { d.Permissions[1].Create = true }, "may only grant read and write"},
		{"row with ifOwner", func(d *DocType) { d.Permissions[1].IfOwner = true }, "may only grant read and write"},
		{"row grants import", func(d *DocType) { d.Permissions[1].Import = true }, "may only grant read and write"},
		{"restricted title", func(d *DocType) { d.TitleField = "salary" }, "titleField \"salary\" has permlevel 1"},
		{"restricted naming field", func(d *DocType) { d.IDGeneration.Field = "salary" }, "idGeneration.field"},
		{"restricted naming format", func(d *DocType) { d.IDGeneration.Format = "{title}-{salary}" }, "idGeneration.format"},
		{"restricted search field", func(d *DocType) { d.SearchFields = []string{"title", "salary"} }, "searchFields"},
		{"restricted link subtitle", func(d *DocType) { d.LinkSubtitle = []string{"salary"} }, "linkSubtitle"},
		{"fetch into level 0", func(d *DocType) {
			d.Fields = append(d.Fields, &Field{Fieldname: "cost_copy", Fieldtype: "Currency", FetchFrom: "source.cost"})
		}, "copies a permlevel 1 field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, d := base()
			tc.mutate(d)
			d.ResetFieldIndex()
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
	t.Run("fetch into restricted field is allowed", func(t *testing.T) {
		r, d := base()
		d.Fields = append(d.Fields, &Field{Fieldname: "cost_copy", Fieldtype: "Currency", FetchFrom: "source.cost", Permlevel: 1})
		d.ResetFieldIndex()
		if err := r.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestSEC02_ExtensionMayGrantALevelToAnExistingRole(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "Target", App: "core", Fields: []*Field{
		{Fieldname: "salary", Fieldtype: "Currency", Permlevel: 1},
	}, Permissions: []Perm{{Role: "HR", Read: true}}})
	err := r.ApplyExtensions([]*Extension{{
		App: "hr", Doctype: "Target", SourceFile: "hr.ts",
		Permissions: []Perm{{Role: "HR", Read: true, Permlevel: 1}},
	}}, Apps{})
	if err != nil {
		t.Fatalf("a level-1 grant to a role granted level 0 must be accepted: %v", err)
	}
	if len(r.DocTypes["Target"].Permissions) != 2 {
		t.Fatalf("permissions = %+v", r.DocTypes["Target"].Permissions)
	}
	err = r.ApplyExtensions([]*Extension{{
		App: "hr2", Doctype: "Target", SourceFile: "hr2.ts",
		Permissions: []Perm{{Role: "HR", Read: true, Permlevel: 1}},
	}}, Apps{})
	if err == nil || !strings.Contains(err.Error(), "at permlevel 1") {
		t.Fatalf("a duplicate level grant must be refused, got %v", err)
	}
}

func TestPermHasImport(t *testing.T) {
	p := Perm{Role: "R", Import: true}
	if !p.Has("import") || p.Has("export") || (Perm{}).Has("import") {
		t.Fatal("import is its own flag")
	}
}
