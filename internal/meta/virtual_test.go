package meta

import (
	"strings"
	"testing"
)

// partyReg is Person ∪ Organization, the shape of issue #17, plus whatever the
// case under test changes on Party.
func partyReg(t *testing.T, mut func(p *DocType)) (*Registry, *DocType) {
	t.Helper()
	r := NewRegistry()
	city := &DocType{Name: "City", Fields: []*Field{{Fieldname: "city_name", Fieldtype: "Data"}}}
	person := &DocType{Name: "Person", Fields: []*Field{
		{Fieldname: "person_name", Fieldtype: "Data"},
		{Fieldname: "cpf", Fieldtype: "Data"},
		{Fieldname: "city", Fieldtype: "Link", Options: "City"},
		{Fieldname: "birth", Fieldtype: "Date"},
		{Fieldname: "secret", Fieldtype: "Password"},
	}}
	org := &DocType{Name: "Organization", Fields: []*Field{
		{Fieldname: "organization_name", Fieldtype: "Data"},
		{Fieldname: "cnpj", Fieldtype: "Data"},
		{Fieldname: "city", Fieldtype: "Link", Options: "City"},
		{Fieldname: "hq", Fieldtype: "Data"},
		{Fieldname: "employees", Fieldtype: "Int"},
	}}
	party := &DocType{Name: "Party", TitleField: "party_name",
		Virtual: &VirtualDef{Sources: []VirtualSource{
			{DocType: "Person", Fields: map[string]string{"party_name": "person_name", "tax_id": "cpf", "city": "city"}},
			{DocType: "Organization", Fields: map[string]string{"party_name": "organization_name", "tax_id": "cnpj", "city": "city"}},
		}},
		Fields: []*Field{
			{Fieldname: "party_name", Fieldtype: "Data"},
			{Fieldname: "tax_id", Fieldtype: "Data"},
			{Fieldname: "city", Fieldtype: "Link", Options: "City"},
		},
		Permissions: []Perm{{Role: "Sales", Read: true, Report: true, Export: true}},
	}
	if mut != nil {
		mut(party)
	}
	for _, d := range []*DocType{city, person, org, party} {
		if err := r.Add(d); err != nil {
			t.Fatalf("add: %v", err)
		}
	}
	r.ApplyVirtual()
	return r, party
}

func TestVirtualValid(t *testing.T) {
	r, party := partyReg(t, nil)
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	f := party.Field(SourceDocTypeField)
	if f == nil || f.Fieldtype != "Data" || !f.InStandardFilter {
		t.Fatalf("source_doctype not injected: %+v", f)
	}
	if party.GloballySearchable() {
		t.Fatal("a virtual DocType joins global search only when it opts in")
	}
	yes := true
	party.GlobalSearch = &yes
	if !party.GloballySearchable() {
		t.Fatal("globalSearch: true must opt it in")
	}
	if got := r.VirtualsOf("Person"); len(got) != 1 || got[0] != party {
		t.Fatalf("VirtualsOf(Person) = %v", got)
	}
	if party.VirtualSource("City") != nil {
		t.Fatal("City is not a source")
	}
}

func TestVirtualKeepsDeclaredSourceField(t *testing.T) {
	r, party := partyReg(t, func(p *DocType) {
		p.Fields = append(p.Fields, &Field{Fieldname: SourceDocTypeField, Fieldtype: "Data", Label: "Kind"})
	})
	if err := r.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if party.Field(SourceDocTypeField).Label != "Kind" {
		t.Fatal("declared source_doctype overwritten")
	}
}

func TestSplitVirtualID(t *testing.T) {
	src, id, ok := SplitVirtualID("Person:abc:def")
	if !ok || src != "Person" || id != "abc:def" {
		t.Fatalf("split at the first separator: %q %q %v", src, id, ok)
	}
	for _, bad := range []string{"", "abc", ":abc", "Person:"} {
		if _, _, ok := SplitVirtualID(bad); ok {
			t.Fatalf("%q split", bad)
		}
	}
	if VirtualID("Person", "abc") != "Person:abc" {
		t.Fatal("VirtualID")
	}
}

func TestVirtualInvalidMetadata(t *testing.T) {
	src := func(p *DocType, i int) *VirtualSource { return &p.Virtual.Sources[i] }
	cases := map[string]struct {
		mut  func(p *DocType)
		want string
	}{
		"single":       {func(p *DocType) { p.IsSingle = true }, "cannot be a child table, a Single"},
		"tree":         {func(p *DocType) { p.IsTree = true }, "cannot be a child table, a Single, a tree"},
		"submittable":  {func(p *DocType) { p.Submittable = true }, "or submittable"},
		"trackChanges": {func(p *DocType) { p.TrackChanges = true }, "no trackChanges"},
		"allowRename":  {func(p *DocType) { p.AllowRename = true }, "no trackChanges or allowRename"},
		"idGeneration": {func(p *DocType) { p.IDGeneration = IDGeneration{Hash: true} }, "idGeneration"},
		"renamedFrom":  {func(p *DocType) { p.RenamedFrom = Names{"Parties"} }, "no renamedFrom"},
		"uniqueKeys":   {func(p *DocType) { p.UniqueKeys = []UniqueKey{{Name: "k", Fields: []string{"tax_id"}}} }, "no table to hold an index"},
		"write perm":   {func(p *DocType) { p.Permissions[0].Write = true }, "grants only read"},
		"create perm":  {func(p *DocType) { p.Permissions[0].Create = true }, "grants only read"},
		"level perm":   {func(p *DocType) { p.Permissions = append(p.Permissions, Perm{Role: "Sales", Permlevel: 1, Read: true}) }, "no field levels"},
		"field level":  {func(p *DocType) { p.Fields[1].Permlevel = 1 }, "no field levels"},
		"unique field": {func(p *DocType) { p.Fields[1].Unique = true }, "need a table"},
		"fetchFrom":    {func(p *DocType) { p.Fields[1].FetchFrom = "city.city_name" }, "need a table"},
		"table field": {func(p *DocType) {
			p.Fields = append(p.Fields, &Field{Fieldname: "rows", Fieldtype: "Table", Options: "City"})
		}, "no child tables"},
		"dynamic link": {func(p *DocType) {
			p.Fields = append(p.Fields, &Field{Fieldname: "ref", Fieldtype: "Dynamic Link", Options: "tax_id"})
		}, "cannot hold a Dynamic Link"},
		"password":      {func(p *DocType) { p.Fields = append(p.Fields, &Field{Fieldname: "pw", Fieldtype: "Password"}) }, "cannot expose a Password"},
		"no sources":    {func(p *DocType) { p.Virtual.Sources = nil }, "at least one source"},
		"duplicate":     {func(p *DocType) { p.Virtual.Sources = append(p.Virtual.Sources, p.Virtual.Sources[0]) }, "declared twice"},
		"missing":       {func(p *DocType) { src(p, 0).DocType = "Nobody" }, `"Nobody" does not exist`},
		"self":          {func(p *DocType) { src(p, 0).DocType = "Party" }, "is a virtual DocType"},
		"unknown key":   {func(p *DocType) { src(p, 0).Fields["nope"] = "cpf" }, `maps "nope", which is not a field of Party`},
		"std key":       {func(p *DocType) { src(p, 0).Fields["id"] = "cpf" }, "which the union fills itself"},
		"source key":    {func(p *DocType) { src(p, 0).Fields[SourceDocTypeField] = "cpf" }, "which the union fills itself"},
		"unknown field": {func(p *DocType) { src(p, 0).Fields["tax_id"] = "rg" }, `to "rg", which is not a field of Person`},
		"type":          {func(p *DocType) { src(p, 0).Fields["tax_id"] = "birth" }, "the column types differ"},
		"link target":   {func(p *DocType) { src(p, 1).Fields["city"] = "hq" }, "which is not a Link to City"},
		"password src":  {func(p *DocType) { src(p, 0).Fields["tax_id"] = "secret" }, "has no readable column"},
		"std type":      {func(p *DocType) { src(p, 0).Fields["tax_id"] = "creation" }, "a timestamptz column"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, _ := partyReg(t, tc.mut)
			err := r.Validate()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
		})
	}
}

func TestVirtualStdColumnMapping(t *testing.T) {
	r, _ := partyReg(t, func(p *DocType) {
		p.Fields = append(p.Fields, &Field{Fieldname: "since", Fieldtype: "Datetime"})
		p.Virtual.Sources[0].Fields["since"] = "creation"
	})
	if err := r.Validate(); err != nil {
		t.Fatalf("a Datetime may read a source's creation: %v", err)
	}
}

func TestVirtualSourceNameWithSeparator(t *testing.T) {
	r := NewRegistry()
	r.Add(&DocType{Name: "A:B", Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}})
	r.Add(&DocType{Name: "V", Virtual: &VirtualDef{Sources: []VirtualSource{{DocType: "A:B", Fields: map[string]string{"x": "x"}}}},
		Fields: []*Field{{Fieldname: "x", Fieldtype: "Data"}}})
	r.ApplyVirtual()
	if err := r.Validate(); err == nil || !strings.Contains(err.Error(), "cannot contain") {
		t.Fatalf("want a refusal, got %v", err)
	}
}
