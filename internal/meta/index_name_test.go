package meta

import (
	"strings"
	"testing"
)

// #144 — Postgres keeps the first 63 bytes of an identifier, so an index name
// past that has to be made to fit by the planner, the same way every time.
func TestIndexName(t *testing.T) {
	if got := IndexName("tab_pessoa", "cpf"); got != "tab_pessoa__cpf" {
		t.Fatalf("a name that fits changed: %q", got)
	}
	if got := LegacyIndexNames("tab_pessoa", "cpf"); len(got) != 1 || got[0] != "tab_pessoa_cpf" {
		t.Fatalf("a name that fits had one earlier name, got %q", got)
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
	full := table + "__" + field
	if !strings.HasPrefix(a, full[:MaxIdentifier-9]+"_") {
		t.Fatalf("%q does not keep the readable prefix", a)
	}
	// What #144 named it, and what Postgres kept of it before that.
	old := table + "_" + field
	legacy := LegacyIndexNames(table, field)
	if len(legacy) != 2 || len(legacy[0]) != MaxIdentifier || !strings.HasPrefix(legacy[0], old[:MaxIdentifier-9]+"_") || legacy[1] != old[:MaxIdentifier] {
		t.Fatalf("earlier names = %q", legacy)
	}
	// Two fields Postgres would have truncated to one and the same name.
	other := field + "_outro"
	if LegacyIndexNames(table, field)[1] != LegacyIndexNames(table, other)[1] {
		t.Fatal("the fixture no longer collides under truncation")
	}
	if b := IndexName(table, other); a == b {
		t.Fatalf("two names collide: %q", a)
	}
}

// #149 — Postgres keeps tables and indexes in one namespace. Product.model's
// index was named tab_product_model, the Product Model table, and Sales'
// order_date index took the name of Sales Order's date index.
func TestIndexNameNeverEqualsATableOrAnotherTablesIndex(t *testing.T) {
	product, model := &DocType{Name: "Product"}, &DocType{Name: "Product Model"}
	if got := IndexName(product.TableName(), "model"); got == model.TableName() {
		t.Fatalf("Product.model's index is named like the Product Model table: %q", got)
	}
	sales, order := &DocType{Name: "Sales"}, &DocType{Name: "Sales Order"}
	if a, b := IndexName(sales.TableName(), "order_date"), IndexName(order.TableName(), "date"); a == b {
		t.Fatalf("two tables' indexes share a name: %q", a)
	}
	// What makes that hold for every name: no table carries "__", the
	// separator of every index name.
	for _, name := range []string{"Product  Model", "Product_Model", "product__model", "Item Já", "_Odd_ Name_", "XMLParser", "aB_C"} {
		if tn := (&DocType{Name: name}).TableName(); strings.Contains(tn, "__") {
			t.Fatalf("%q gives the table %q", name, tn)
		}
	}
}

func TestPrimaryKeyNameIsPostgres(t *testing.T) {
	if got := PrimaryKeyName("tab_pessoa"); got != "tab_pessoa_pkey" {
		t.Fatalf("got %q", got)
	}
	// Postgres cuts the table's part short so that "_pkey" still fits.
	long := "tab_" + strings.Repeat("x", MaxIdentifier-4)
	if got := PrimaryKeyName(long); len(got) != MaxIdentifier || got != long[:MaxIdentifier-5]+"_pkey" {
		t.Fatalf("got %q", got)
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
