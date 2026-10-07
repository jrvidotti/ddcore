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

func TestTenancySpacesKeepTenantOnlyDocTypesFromThePlatform(t *testing.T) {
	r := tenancyRegistry(t,
		&DocType{Name: "Folha", App: "demo", Space: SpaceTenant, Fields: []*Field{{Fieldname: "linhas", Fieldtype: "Table", Options: "Linha"}}},
		&DocType{Name: "Linha", App: "demo", IsChild: true},
		&DocType{Name: "Pessoa", App: "demo"},
		&DocType{Name: "Ponto", App: "rh"},
		&DocType{Name: "Aviso", App: "rh", Space: SpaceAny},
		&DocType{Name: "Cargo", App: "rh", Shared: true},
		&DocType{Name: "Tudo", App: "rh", Virtual: &VirtualDef{}},
	)
	apps := map[string]string{"rh": SpaceTenant}
	r.ApplyTenancy(true)
	r.ApplySpaces(apps)
	want := map[string]bool{"Folha": true, "Linha": false, "Pessoa": false, "Ponto": true, "Aviso": false, "Cargo": false, "Tudo": false}
	for name, only := range want {
		if d, _ := r.Get(name); d.TenantOnly != only {
			t.Errorf("%s: TenantOnly = %v, want %v", name, d.TenantOnly, only)
		}
	}
	r.ApplyTenancy(false)
	r.ApplySpaces(apps)
	for name := range want {
		if d, _ := r.Get(name); d.TenantOnly {
			t.Errorf("%s: tenant-only with tenancy off", name)
		}
	}
}

func TestTenancySpaceValues(t *testing.T) {
	for want, d := range map[string]*DocType{
		`space "Tenant" is neither`:               {Name: "A", Space: "Tenant"},
		"a child DocType has no space":            {Name: "B", IsChild: true, Space: SpaceTenant},
		"a virtual DocType has no space":          {Name: "C", Virtual: &VirtualDef{}, Space: SpaceAny},
		"a shared DocType is read in every space": {Name: "D", Shared: true, Space: SpaceTenant},
	} {
		r := tenancyRegistry(t, d)
		// a typo fails without tenancy too
		r.ApplyTenancy(false)
		if err := r.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", d.Name, err, want)
		}
	}
	r := tenancyRegistry(t, &DocType{Name: "E", Shared: true, Space: SpaceAny})
	r.ApplyTenancy(true)
	if err := r.Validate(); err != nil {
		t.Fatalf("shared with space any: %v", err)
	}
}
