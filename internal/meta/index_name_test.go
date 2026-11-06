package meta

import (
	"strings"
	"testing"
)

// #144 — Postgres keeps the first 63 bytes of an identifier, so an index name
// past that has to be made to fit by the planner, the same way every time.
func TestIndexName(t *testing.T) {
	if got := IndexName("tab_pessoa", "cpf"); got != "tab_pessoa_cpf" {
		t.Fatalf("a name that fits changed: %q", got)
	}
	if got := TruncatedIndexName("tab_pessoa", "cpf"); got != "" {
		t.Fatalf("a name that fits has nothing to adopt, got %q", got)
	}

	table := "tab_tag_one_produto_estoque_referencia"
	field := "codigo_produto_estoque_referencia"
	a := IndexName(table, field)
	if len(a) != MaxIdentifier {
		t.Fatalf("%q is %d bytes, want %d", a, len(a), MaxIdentifier)
	}
	if a != IndexName(table, field) {
		t.Fatal("the same name hashed twice gave two answers")
	}
	full := table + "_" + field
	if !strings.HasPrefix(a, full[:MaxIdentifier-9]+"_") {
		t.Fatalf("%q does not keep the readable prefix", a)
	}
	if got := TruncatedIndexName(table, field); got != full[:MaxIdentifier] {
		t.Fatalf("truncated name = %q", got)
	}
	// Two fields Postgres would have truncated to one and the same name.
	other := field + "_outro"
	if TruncatedIndexName(table, field) != TruncatedIndexName(table, other) {
		t.Fatal("the fixture no longer collides under truncation")
	}
	if b := IndexName(table, other); a == b {
		t.Fatalf("two names collide: %q", a)
	}
}

// A table or a column past the limit fails the way an index did, and cannot be
// shortened behind the author's back: those names are the app's API.
func TestValidateRefusesIdentifiersPostgresWouldTruncate(t *testing.T) {
	long := strings.Repeat("x", MaxIdentifier+1)
	cases := []struct {
		name, want string
		d          *DocType
	}{
		{"fieldname", "past the 63 Postgres keeps",
			&DocType{Name: "Invoice", Fields: []*Field{{Fieldname: long, Fieldtype: "Data", Label: "X"}}}},
		{"table", "its table name",
			&DocType{Name: strings.TrimSpace(strings.Repeat("Word ", 13)), Fields: []*Field{{Fieldname: "a", Fieldtype: "Data", Label: "A"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRegistry()
			r.Add(tc.d)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("wanted an error mentioning %q, got %v", tc.want, err)
			}
		})
	}
	// Exactly at the limit is fine.
	r := NewRegistry()
	r.Add(&DocType{Name: "Invoice", Fields: []*Field{{Fieldname: strings.Repeat("x", MaxIdentifier), Fieldtype: "Data", Label: "X"}}})
	if err := r.Validate(); err != nil {
		t.Fatalf("a 63-byte fieldname was refused: %v", err)
	}
}
