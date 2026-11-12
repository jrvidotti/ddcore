package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

func minDateRegistry(end *Field) *Registry {
	r := NewRegistry()
	r.Add(&DocType{Name: "Period", Fields: []*Field{
		{Fieldname: "start_date", Fieldtype: "Date"},
		{Fieldname: "starts_at", Fieldtype: "Datetime"},
		end,
	}, Permissions: []Perm{{Role: "System Manager", Read: true}}})
	return r
}

func TestMinDateFieldValidation(t *testing.T) {
	if err := minDateRegistry(&Field{Fieldname: "end_date", Fieldtype: "Date", MinDateField: "start_date"}).Validate(); err != nil {
		t.Fatalf("a Date after another Date: %v", err)
	}
	for _, c := range []struct {
		f    *Field
		want string
	}{
		{&Field{Fieldname: "end_date", Fieldtype: "Data", MinDateField: "start_date"}, "minDateField is for a Date field, not a Data"},
		{&Field{Fieldname: "end_date", Fieldtype: "Date", MinDateField: "end_date"}, "minDateField names the field itself"},
		{&Field{Fieldname: "end_date", Fieldtype: "Date", MinDateField: "nope"}, `minDateField "nope" is not a field of "Period"`},
		{&Field{Fieldname: "end_date", Fieldtype: "Date", MinDateField: "starts_at"}, `minDateField "starts_at" is a Datetime, not a Date`},
	} {
		err := minDateRegistry(c.f).Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.f, c.want, err)
		}
	}
}

// the desk reads it: the meta carries it, and another app may set it
func TestMinDateFieldReachesTheDesk(t *testing.T) {
	var f Field
	if err := json.Unmarshal([]byte(`{"fieldname":"end_date","fieldtype":"Date","minDateField":"start_date"}`), &f); err != nil || f.MinDateField != "start_date" {
		t.Fatalf("minDateField not parsed: %+v, %v", f, err)
	}
	out, _ := json.Marshal(f)
	if !strings.Contains(string(out), `"minDateField":"start_date"`) {
		t.Fatalf("minDateField not served: %s", out)
	}
	if !FieldProps["minDateField"] {
		t.Fatal("minDateField is not an extensible field property")
	}
}
