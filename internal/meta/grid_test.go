package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

func gridRegistry(parent ...*Field) *Registry {
	r := NewRegistry()
	r.Add(&DocType{Name: "Course Student", IsChild: true, Fields: []*Field{
		{Fieldname: "employee_name", Fieldtype: "Data"},
		{Fieldname: "grade", Fieldtype: "Float", Computed: true},
		{Fieldname: "sec", Fieldtype: "Section Break"},
	}})
	r.Add(&DocType{Name: "Course", Fields: append([]*Field{{Fieldname: "unit", Fieldtype: "Data"}}, parent...)})
	return r
}

func TestFieldGridPropsRoundTripThroughJSON(t *testing.T) {
	in := `{"fieldtype":"Table","gridSort":{"field":"employee_name","order":"desc"},"gridSortable":true,"gridExport":true,"gridSelect":true}`
	var f Field
	if err := json.Unmarshal([]byte(in), &f); err != nil {
		t.Fatal(err)
	}
	if f.GridSort == nil || f.GridSort.Field != "employee_name" || f.GridSort.Order != "desc" || !f.GridSortable || !f.GridExport || !f.GridSelect {
		t.Fatalf("parsed %+v", f)
	}
	b, _ := json.Marshal(f)
	if string(b) != in {
		t.Fatalf("json=%s", b)
	}
}

func TestGridPropsValidation(t *testing.T) {
	ok := []*Field{
		{Fieldname: "students", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "employee_name"}, GridSortable: true, GridExport: true, GridSelect: true},
		{Fieldname: "by_idx", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "idx", Order: "desc"}},
		{Fieldname: "by_grade", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "grade"}},
		{Fieldname: "attendance", Fieldtype: "Report", Options: "Course Attendance", ReportFilters: map[string]string{"course": "id", "unit": "unit"}, GridSort: &GridSort{Field: "whatever"}, GridExport: true},
	}
	if err := gridRegistry(ok...).Validate(); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		f    *Field
		want string
	}{
		{&Field{Fieldname: "s", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "nope"}}, `gridSort.field "nope" is not a field`},
		{&Field{Fieldname: "s", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "sec"}}, `gridSort.field "sec" is not a field`},
		{&Field{Fieldname: "s", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{Field: "employee_name", Order: "up"}}, "must be asc or desc"},
		{&Field{Fieldname: "s", Fieldtype: "Table", Options: "Course Student", GridSort: &GridSort{}}, "gridSort needs a field"},
		{&Field{Fieldname: "d", Fieldtype: "Data", GridExport: true}, "for a Table or a Report field, not a Data"},
		{&Field{Fieldname: "d", Fieldtype: "Data", ReportFilters: map[string]string{"a": "id"}}, "reportFilters is for a Report field"},
		{&Field{Fieldname: "r", Fieldtype: "Report"}, "needs options naming the report"},
		{&Field{Fieldname: "Bad Name", Fieldtype: "Report", Options: "X"}, `invalid fieldname "Bad Name" on a Report field`},
		{&Field{Fieldname: "r", Fieldtype: "Report", Options: "X", ReportFilters: map[string]string{"course": "missing"}}, `reportFilters.course takes "missing"`},
	} {
		err := gridRegistry(c.f).Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.f, c.want, err)
		}
	}
}

func TestComputedFieldHasNoColumn(t *testing.T) {
	r := gridRegistry()
	child := r.DocTypes["Course Student"]
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if child.HasColumn("grade") || !child.HasColumn("employee_name") {
		t.Fatal("a computed field must not be a column")
	}
	for _, f := range child.DataFields() {
		if f.Fieldname == "grade" {
			t.Fatal("DataFields must skip a computed field")
		}
	}
}

func TestComputedFieldValidation(t *testing.T) {
	for _, c := range []struct {
		f    *Field
		want string
	}{
		{&Field{Fieldname: "t", Fieldtype: "Table", Options: "Course Student", Computed: true}, "a Table cannot be computed"},
		{&Field{Fieldname: "p", Fieldtype: "Password", Computed: true}, "a Password cannot be computed"},
		{&Field{Fieldname: "x", Fieldtype: "Data", Computed: true, Reqd: true}, "cannot be fetchFrom, unique or reqd"},
		{&Field{Fieldname: "x", Fieldtype: "Data", Computed: true, Unique: true}, "cannot be fetchFrom, unique or reqd"},
	} {
		err := gridRegistry(c.f).Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.f, c.want, err)
		}
	}
}
