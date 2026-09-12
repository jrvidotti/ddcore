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
		t.Fatal("expected error for link to non-existent DocType")
	}
	r.Add(&DocType{Name: "B", Fields: []*Field{{Fieldname: "y", Fieldtype: "Data"}}})
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestVaultFieldtype(t *testing.T) {
	if ColumnType("Vault") != "" {
		t.Fatalf("expected ColumnType(Vault) == \"\", got %q", ColumnType("Vault"))
	}
	r := NewRegistry()
	d := &DocType{
		Name: "Conta",
		Fields: []*Field{
			{Fieldname: "titulo", Fieldtype: "Data"},
			{Fieldname: "api_token", Fieldtype: "Vault", Options: "asaas:token:{name}"},
		},
	}
	r.Add(d)
	if err := r.Validate(); err != nil {
		t.Fatalf("validation failed for DocType with Vault field: %v", err)
	}

	dfs := d.DataFields()
	if len(dfs) != 1 || dfs[0].Fieldname != "titulo" {
		t.Fatalf("expected DataFields to exclude Vault field, got %v", dfs)
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

func TestFieldWidthRoundTripsThroughJSON(t *testing.T) {
	var field Field
	if err := json.Unmarshal([]byte(`{"fieldtype":"Percent","width":"sm"}`), &field); err != nil {
		t.Fatal(err)
	}
	if field.Width != "sm" {
		t.Fatalf("Width=%q want sm", field.Width)
	}
	b, err := json.Marshal(field)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"fieldtype":"Percent","width":"sm"}` {
		t.Fatalf("json=%s", b)
	}
}

func TestFieldWidthValidation(t *testing.T) {
	for _, w := range []string{"sm", "md", "lg", "full"} {
		r := NewRegistry()
		r.Add(&DocType{Name: "Doc", Fields: []*Field{{Fieldname: "pct", Fieldtype: "Percent", Width: w}}})
		if err := r.Validate(); err != nil {
			t.Fatalf("width %q should be valid: %v", w, err)
		}
	}

	r := NewRegistry()
	r.Add(&DocType{Name: "Doc", Fields: []*Field{{Fieldname: "pct", Fieldtype: "Percent", Width: "xl"}}})
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "invalid width") {
		t.Fatalf("expected invalid width error, got %v", err)
	}
}

func TestColumnBreakIsDroppedWithAWarning(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "Contract", Fields: []*Field{
		{Fieldname: "title", Fieldtype: "Data"},
		{Fieldtype: "Column Break"},
		{Fieldname: "start", Fieldtype: "Date"},
		{Fieldtype: "Column Break"},
	}})

	warnings := r.DropObsoleteFields()
	if len(warnings) != 1 || !strings.Contains(warnings[0], "Contract") || !strings.Contains(warnings[0], "dropped 2") {
		t.Fatalf("warnings=%v", warnings)
	}

	d, _ := r.Get("Contract")
	if len(d.Fields) != 2 || d.Fields[0].Fieldname != "title" || d.Fields[1].Fieldname != "start" {
		t.Fatalf("fields=%+v", d.Fields)
	}
	// dropping is what lets an app written against an older ddcore still load
	if err := r.Validate(); err != nil {
		t.Fatalf("validate after dropping: %v", err)
	}
}

func TestColumnBreakIsNoLongerAValidFieldtype(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "Doc", Fields: []*Field{{Fieldtype: "Column Break"}}})
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "invalid fieldtype") {
		t.Fatalf("expected invalid fieldtype error, got %v", err)
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

// numeric(21,9) cannot hold more than nine decimal places, so a field asking
// for more is a developer error and belongs at migrate rather than in a silent
// truncation on the first save.
func TestPrecisionBeyondTheColumnIsRefused(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "Fatura", Fields: []*Field{
		{Fieldname: "total", Fieldtype: "Currency", Label: "Total", Precision: 12},
	}})
	err := r.Validate()
	if err == nil {
		t.Fatal("precision 12 was accepted")
	}
	if !strings.Contains(err.Error(), "precision") {
		t.Fatalf("error does not name the problem: %v", err)
	}

	ok := NewRegistry()
	ok.Add(&DocType{Name: "Fatura", Fields: []*Field{
		{Fieldname: "total", Fieldtype: "Currency", Label: "Total", Precision: 9},
		{Fieldname: "peso", Fieldtype: "Float", Label: "Peso"},
	}})
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid precision was refused: %v", err)
	}
}

// DAT-05 — a compound business key becomes a partial unique index, so every
// way it could fail to become one is refused at load. A declaration that
// silently enforces nothing is worse than no declaration at all.
func TestValidateRejectsBadUniqueKeys(t *testing.T) {
	fields := func() []*Field {
		return []*Field{
			{Fieldname: "customer", Fieldtype: "Data", Label: "Customer"},
			{Fieldname: "invoice_no", Fieldtype: "Data", Label: "Invoice no"},
			{Fieldname: "lines", Fieldtype: "Table", Options: "Invoice Line"},
			{Fieldname: "layout", Fieldtype: "Section Break"},
		}
	}
	long := strings.Repeat("x", 60)
	cases := []struct {
		name, want string
		keys       []UniqueKey
		mutate     func(*DocType)
	}{
		{"unknown field", `field "gone" does not exist`,
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "gone"}}}, nil},
		{"standard column", "is a standard column",
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "owner"}}}, nil},
		{"child table", "which has no column",
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "lines"}}}, nil},
		{"layout field", "which has no column",
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "layout"}}}, nil},
		{"one field", "spans 1 field",
			[]UniqueKey{{Name: "k", Fields: []string{"customer"}}}, nil},
		{"no fields", "spans 0 field",
			[]UniqueKey{{Name: "k"}}, nil},
		{"repeated field", `lists field "customer" twice`,
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "customer"}}}, nil},
		{"duplicate key name", `"k" is declared twice`, []UniqueKey{
			{Name: "k", Fields: []string{"customer", "invoice_no"}},
			{Name: "k", Fields: []string{"invoice_no", "customer"}},
		}, nil},
		// Order decides the index, never the constraint, so the same fields in
		// either order are one key wearing two names.
		{"same fields", "cover the same fields", []UniqueKey{
			{Name: "a", Fields: []string{"customer", "invoice_no"}},
			{Name: "b", Fields: []string{"invoice_no", "customer"}},
		}, nil},
		{"bad name", "not a valid name",
			[]UniqueKey{{Name: "Customer Key", Fields: []string{"customer", "invoice_no"}}}, nil},
		{"name too long", "past the 63 Postgres keeps",
			[]UniqueKey{{Name: long, Fields: []string{"customer", "invoice_no"}}}, nil},
		// A field of that name would want the very same index name.
		{"name a field already claims", "would claim the same index name",
			[]UniqueKey{{Name: "extra", Fields: []string{"customer", "invoice_no"}}},
			func(d *DocType) {
				d.Fields = append(d.Fields, &Field{Fieldname: "uk_extra", Fieldtype: "Data"})
			}},
		{"child doctype", "a child DocType has no business key of its own",
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "invoice_no"}}},
			func(d *DocType) { d.IsChild = true }},
		{"single doctype", "there is nothing to keep unique",
			[]UniqueKey{{Name: "k", Fields: []string{"customer", "invoice_no"}}},
			func(d *DocType) { d.IsSingle = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := &DocType{Name: "Invoice", Fields: fields(), UniqueKeys: tc.keys}
			if tc.mutate != nil {
				tc.mutate(d)
			}
			r := NewRegistry()
			r.Add(d)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
}

// The declaration a reader would actually write has to load, and two keys over
// different field sets are two indexes, not a conflict.
func TestValidateAcceptsUniqueKeys(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "Invoice", UniqueKeys: []UniqueKey{
		{Name: "customer_invoice_no", Fields: []string{"customer", "invoice_no"}},
		{Name: "customer_period", Fields: []string{"customer", "period"}},
	}, Fields: []*Field{
		{Fieldname: "customer", Fieldtype: "Link", Options: "Customer", Label: "Customer"},
		{Fieldname: "invoice_no", Fieldtype: "Data", Label: "Invoice no"},
		{Fieldname: "period", Fieldtype: "Month", Label: "Period"},
	}})
	r.Add(&DocType{Name: "Customer", Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}})
	if err := r.Validate(); err != nil {
		t.Fatalf("a well-formed pair of keys must load: %v", err)
	}
}
