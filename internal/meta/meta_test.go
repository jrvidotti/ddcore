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

// numeric(21,9) cannot hold more than nine decimal places, so a field asking
// for more is a developer error and belongs at migrate rather than in a silent
// truncation on the first save.
func TestPrecisionBeyondTheColumnIsRefused(t *testing.T) {
	r := &Registry{DocTypes: map[string]*DocType{
		"Fatura": {Name: "Fatura", Fields: []*Field{
			{Fieldname: "total", Fieldtype: "Currency", Label: "Total", Precision: 12},
		}},
	}}
	err := r.Validate()
	if err == nil {
		t.Fatal("precision 12 was accepted")
	}
	if !strings.Contains(err.Error(), "precision") {
		t.Fatalf("error does not name the problem: %v", err)
	}

	ok := &Registry{DocTypes: map[string]*DocType{
		"Fatura": {Name: "Fatura", Fields: []*Field{
			{Fieldname: "total", Fieldtype: "Currency", Label: "Total", Precision: 9},
			{Fieldname: "peso", Fieldtype: "Float", Label: "Peso"},
		}},
	}}
	if err := ok.Validate(); err != nil {
		t.Fatalf("a valid precision was refused: %v", err)
	}
}
