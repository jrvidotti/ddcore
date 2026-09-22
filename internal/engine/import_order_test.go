package engine

import (
	"reflect"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func dt(name string, fields ...*meta.Field) *meta.DocType {
	return &meta.DocType{Name: name, Fields: fields}
}

func link(fieldname, target string) *meta.Field {
	return &meta.Field{Fieldname: fieldname, Fieldtype: "Link", Options: target}
}

func table(fieldname, child string) *meta.Field {
	return &meta.Field{Fieldname: fieldname, Fieldtype: "Table", Options: child}
}

func lookupOf(dts ...*meta.DocType) func(string) (*meta.DocType, error) {
	byName := map[string]*meta.DocType{}
	for _, d := range dts {
		byName[d.Name] = d
	}
	return func(n string) (*meta.DocType, error) {
		d, ok := byName[n]
		if !ok {
			return nil, cerr.NotFound("no DocType %s", n)
		}
		return d, nil
	}
}

func TestImportOrderPutsATargetBeforeItsLink(t *testing.T) {
	lookup := lookupOf(
		dt("Project", link("manager", "User")),
		dt("Task", link("project", "Project")),
		dt("User"),
	)
	order, err := ImportOrder([]string{"Task", "Project", "User"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range order {
		names = append(names, s.Doctype)
	}
	if !reflect.DeepEqual(names, []string{"User", "Project", "Task"}) {
		t.Fatalf("order = %v", names)
	}
	for _, s := range order {
		if len(s.Deferred) != 0 {
			t.Fatalf("%s deferred %v with no cycle in sight", s.Doctype, s.Deferred)
		}
	}
}

// A self link (amended_from is one) can never be satisfied at insert time.
func TestImportOrderDefersASelfLink(t *testing.T) {
	lookup := lookupOf(dt("Pedido", link("amended_from", "Pedido")))
	order, err := ImportOrder([]string{"Pedido"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 1 || !reflect.DeepEqual(order[0].Deferred, []string{"amended_from"}) {
		t.Fatalf("order = %+v", order)
	}
}

// Two DocTypes that link to each other have no valid order; one side's link
// waits for the finalize pass instead of failing the load.
func TestImportOrderBreaksACycle(t *testing.T) {
	lookup := lookupOf(
		dt("Invoice", link("customer", "Customer")),
		dt("Customer", link("last_invoice", "Invoice")),
	)
	order, err := ImportOrder([]string{"Invoice", "Customer"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 2 {
		t.Fatalf("order = %+v", order)
	}
	deferred := 0
	for _, s := range order {
		deferred += len(s.Deferred)
	}
	if deferred != 1 {
		t.Fatalf("exactly one link should be deferred, got %+v", order)
	}
}

// A Dynamic Link's target is a value, not a declaration, so it is always
// checked after the load.
func TestImportOrderAlwaysDefersADynamicLink(t *testing.T) {
	lookup := lookupOf(dt("Comment",
		&meta.Field{Fieldname: "reference_doctype", Fieldtype: "Link", Options: "DocType"},
		&meta.Field{Fieldname: "reference_id", Fieldtype: "Dynamic Link", Options: "reference_doctype"},
	))
	order, err := ImportOrder([]string{"Comment"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order[0].Deferred, []string{"reference_id"}) {
		t.Fatalf("deferred = %v", order[0].Deferred)
	}
}

// A child table's Link counts as the parent's dependency: the rows are written
// with the parent.
func TestImportOrderFollowsChildTableLinks(t *testing.T) {
	lookup := lookupOf(
		dt("Order", table("lines", "Order Line")),
		dt("Order Line", link("product", "Product")),
		dt("Product"),
	)
	order, err := ImportOrder([]string{"Order", "Product"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if order[0].Doctype != "Product" {
		t.Fatalf("order = %+v", order)
	}
}

// A link to something the export does not carry is checked inline against
// whatever the site already holds.
func TestImportOrderIgnoresLinksOutsideTheSet(t *testing.T) {
	lookup := lookupOf(dt("Task", link("assignee", "User")), dt("User"))
	order, err := ImportOrder([]string{"Task"}, lookup)
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 1 || len(order[0].Deferred) != 0 {
		t.Fatalf("order = %+v", order)
	}
}
