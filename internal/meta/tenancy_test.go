package meta

import (
	"strings"
	"testing"
)

func tenancyRegistry(t *testing.T, docs ...*DocType) *Registry {
	t.Helper()
	r := NewRegistry()
	for _, d := range docs {
		if err := r.Add(d); err != nil {
			t.Fatal(err)
		}
	}
	return r
}

func TestTenancyOwnsEveryDocTypeThatIsNotShared(t *testing.T) {
	r := tenancyRegistry(t,
		&DocType{Name: "Order", Fields: []*Field{{Fieldname: "items", Fieldtype: "Table", Options: "Order Item"}}},
		&DocType{Name: "Order Item", IsChild: true},
		&DocType{Name: "Country", Shared: true, Fields: []*Field{{Fieldname: "names", Fieldtype: "Table", Options: "Country Name"}}},
		&DocType{Name: "Country Name", IsChild: true},
		&DocType{Name: "Loose Row", IsChild: true},
		&DocType{Name: "Everything", Virtual: &VirtualDef{}},
	)
	r.ApplyTenancy(true)
	want := map[string]bool{"Order": true, "Order Item": true, "Country": false, "Country Name": false, "Loose Row": true, "Everything": false}
	for name, owned := range want {
		if d, _ := r.Get(name); d.TenantOwned != owned {
			t.Errorf("%s: TenantOwned = %v, want %v", name, d.TenantOwned, owned)
		}
	}

	r.ApplyTenancy(false)
	for name := range want {
		if d, _ := r.Get(name); d.TenantOwned {
			t.Errorf("%s: tenant-owned with tenancy off", name)
		}
	}
}

func TestTenancyReservesTheColumnName(t *testing.T) {
	d := &DocType{Name: "Order", Fields: []*Field{{Fieldname: "tenant", Fieldtype: "Data"}}}
	r := tenancyRegistry(t, d)
	r.ApplyTenancy(false)
	if err := r.Validate(); err != nil {
		t.Fatalf("tenancy off: %v", err)
	}
	r.ApplyTenancy(true)
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), `fieldname "tenant" is reserved`) {
		t.Fatalf("tenancy on: %v", err)
	}
}

func TestTenancyRefusesASharedDocTypeThatReachesIntoATenant(t *testing.T) {
	r := tenancyRegistry(t,
		&DocType{Name: "Customer"},
		&DocType{Name: "Country", Shared: true, Fields: []*Field{{Fieldname: "best", Fieldtype: "Link", Options: "Customer"}}},
	)
	r.ApplyTenancy(true)
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "a shared DocType cannot link to") {
		t.Fatalf("got %v", err)
	}

	r = tenancyRegistry(t,
		&DocType{Name: "Customer", Fields: []*Field{{Fieldname: "rows", Fieldtype: "Table", Options: "Note"}}},
		&DocType{Name: "Country", Shared: true, Fields: []*Field{{Fieldname: "rows", Fieldtype: "Table", Options: "Note"}}},
		&DocType{Name: "Note", IsChild: true},
	)
	r.ApplyTenancy(true)
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "both a shared and a tenant-owned") {
		t.Fatalf("got %v", err)
	}
}
