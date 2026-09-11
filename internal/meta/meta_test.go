package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSnake(t *testing.T) {
	for in, want := range map[string]string{"Reajuste Contrato": "reajuste_contrato", "Imovel": "imovel", "HasRole": "has_role", "User": "user"} {
		if got := Snake(in); got != want {
			t.Errorf("Snake(%q)=%q want %q", in, got, want)
		}
	}
}
func TestValidate(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "A", Fields: []*Field{{Fieldname: "x", Fieldtype: "Link", Options: "B"}}})
	if err := r.Validate(); err == nil {
		t.Fatal("esperava erro de link para DocType inexistente")
	}
	r.Add(&DocType{Name: "B", Fields: []*Field{{Fieldname: "y", Fieldtype: "Data"}}})
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFieldGridEditModeRoundTripsThroughJSON(t *testing.T) {
	var field Field
	if err := json.Unmarshal([]byte(`{"fieldtype":"Table","gridEditMode":"dialog"}`), &field); err != nil {
		t.Fatal(err)
	}
	if field.GridEditMode != "dialog" {
		t.Fatalf("GridEditMode=%q want dialog", field.GridEditMode)
	}
	b, err := json.Marshal(field)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"fieldtype":"Table","gridEditMode":"dialog"}` {
		t.Fatalf("json=%s", b)
	}
}

func TestEmailIsATextFieldtype(t *testing.T) {
	if got := ColumnType("Email"); got != "text" {
		t.Fatalf("ColumnType(Email)=%q want text", got)
	}
	r := NewRegistry()
	r.Add(&DocType{Name: "Contact", Fields: []*Field{{Fieldname: "email", Fieldtype: "Email"}}})
	if err := r.Validate(); err != nil {
		t.Fatalf("Email fieldtype should be valid: %v", err)
	}
}

func TestMonthIsADateFieldtype(t *testing.T) {
	if got := ColumnType("Month"); got != "date" {
		t.Fatalf("ColumnType(Month)=%q want date", got)
	}
	r := NewRegistry()
	r.Add(&DocType{Name: "Lancamento", Fields: []*Field{{Fieldname: "competencia", Fieldtype: "Month"}}})
	if err := r.Validate(); err != nil {
		t.Fatalf("Month fieldtype should be valid: %v", err)
	}
}

// renamedFrom and convert are permanent declarations in the meta, so a mistake
// in one lives on. Validate refuses them at load, before any DDL is planned.

func TestRenamedFromAcceptsAStringOrAChain(t *testing.T) {
	var f Field
	if err := json.Unmarshal([]byte(`{"fieldtype":"Data","renamedFrom":"old"}`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.RenamedFrom) != 1 || f.RenamedFrom[0] != "old" {
		t.Fatalf("string form: %v", f.RenamedFrom)
	}
	f = Field{}
	if err := json.Unmarshal([]byte(`{"fieldtype":"Data","renamedFrom":["a","b"]}`), &f); err != nil {
		t.Fatal(err)
	}
	if len(f.RenamedFrom) != 2 || f.RenamedFrom[1] != "b" {
		t.Fatalf("list form: %v", f.RenamedFrom)
	}
	f = Field{}
	if err := json.Unmarshal([]byte(`{"fieldtype":"Data","renamedFrom":7}`), &f); err == nil {
		t.Fatal("a number is not a fieldname")
	}
}

func TestValidateRejectsBadRenamedFrom(t *testing.T) {
	cases := []struct {
		name, want string
		doctype    *DocType
	}{
		{"itself", "names the field itself", &DocType{Name: "A", Fields: []*Field{
			{Fieldname: "x", Fieldtype: "Data", RenamedFrom: Names{"x"}}}}},
		{"reserved", "reserved column", &DocType{Name: "A", Fields: []*Field{
			{Fieldname: "x", Fieldtype: "Data", RenamedFrom: Names{"docstatus"}}}}},
		{"two claimants", "both declare renamedFrom", &DocType{Name: "A", Fields: []*Field{
			{Fieldname: "x", Fieldtype: "Data", RenamedFrom: Names{"old"}},
			{Fieldname: "y", Fieldtype: "Data", RenamedFrom: Names{"old"}}}}},
		{"swap", "swap names", &DocType{Name: "A", Fields: []*Field{
			{Fieldname: "x", Fieldtype: "Data", RenamedFrom: Names{"y"}},
			{Fieldname: "y", Fieldtype: "Data", RenamedFrom: Names{"x"}}}}},
		{"doctype itself", "names the DocType itself", &DocType{Name: "A", RenamedFrom: Names{"A"},
			Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}}},
		{"same table", "the same table", &DocType{Name: "My Thing", RenamedFrom: Names{"MyThing"},
			Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			r.Add(tc.doctype)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

func TestValidateRejectsBadConvert(t *testing.T) {
	cases := []struct {
		name, want string
		field      *Field
	}{
		{"unknown fieldtype", "is not a fieldtype",
			&Field{Fieldname: "x", Fieldtype: "Currency", Convert: &Convert{From: "Money"}}},
		{"no column", "has no column",
			&Field{Fieldname: "x", Fieldtype: "Currency", Convert: &Convert{From: "Section Break"}}},
		{"nothing to convert", "nothing to convert",
			&Field{Fieldname: "x", Fieldtype: "Data", Convert: &Convert{From: "Select"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			r.Add(&DocType{Name: "A", Fields: []*Field{tc.field}})
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

// Renaming a → b while a new field takes the freed name is legal, and is the
// one shape that looks like a swap but is not: the rename runs first.
func TestValidateAllowsReusingTheFreedName(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "A", Fields: []*Field{
		{Fieldname: "b", Fieldtype: "Data", RenamedFrom: Names{"a"}},
		{Fieldname: "a", Fieldtype: "Int"},
	}})
	if err := r.Validate(); err != nil {
		t.Fatalf("reusing a freed name should be allowed: %v", err)
	}
}

// The half of a rename the declaration cannot do: titleField, sortField and
// searchFields name a field by string, and a rename leaves them dangling.
func TestValidateRejectsDanglingFieldReferences(t *testing.T) {
	cases := []struct {
		name, want string
		doctype    *DocType
	}{
		{"titleField", "titleField", &DocType{Name: "A", TitleField: "gone",
			Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}}},
		{"sortField", "sortField", &DocType{Name: "A", SortField: "gone",
			Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}}},
		{"searchFields", "searchFields", &DocType{Name: "A", SearchFields: []string{"gone"},
			Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			r.Add(tc.doctype)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
	// All three may name a standard column, which is not in Fields:
	// `titleField: "name"` and `searchFields: ["name"]` are both documented.
	r := NewRegistry()
	r.Add(&DocType{Name: "A", SortField: "modified", TitleField: "name", SearchFields: []string{"name", "x"},
		Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}})
	if err := r.Validate(); err != nil {
		t.Fatalf("a standard column is not a dangling reference: %v", err)
	}
}
