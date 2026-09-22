package meta

import (
	"encoding/json"
	"strings"
	"testing"
)

// hostRegistry is a two-app site: `core` owns Role, `crm` owns Lead, and `billing`
// is the app doing the extending in most tests below.
func hostRegistry() *Registry {
	r := NewRegistry()
	r.Add(&DocType{Name: "Role", App: "core", Fields: []*Field{
		{Fieldname: "role_name", Fieldtype: "Data", Label: "Role name"},
	}})
	r.Add(&DocType{Name: "Lead", App: "crm", Label: "Lead", TitleField: "title", Fields: []*Field{
		{Fieldname: "title", Fieldtype: "Data", Label: "Title"},
		{Fieldname: "status", Fieldtype: "Select", Label: "Status", Options: []any{"Open", "Won"}},
	}, Permissions: []Perm{{Role: "Sales", Read: true, Write: true}}})
	r.Add(&DocType{Name: "Lead Note", App: "crm", IsChild: true, Fields: []*Field{
		{Fieldname: "text", Fieldtype: "Text"},
	}})
	return r
}

func apps() Apps {
	return Apps{
		Order:    []string{"core", "crm", "billing"},
		Requires: map[string][]string{"billing": {"crm"}},
	}
}

func ext(app, doctype string, body string) *Extension {
	e := &Extension{}
	if err := json.Unmarshal([]byte(body), e); err != nil {
		panic(err)
	}
	e.App = app
	e.Doctype = doctype
	e.SourceFile = app + ".extensions." + strings.ToLower(doctype)
	return e
}

func apply(t *testing.T, r *Registry, exts ...*Extension) error {
	t.Helper()
	return r.ApplyExtensions(exts, apps())
}

func mustApply(t *testing.T, r *Registry, exts ...*Extension) {
	t.Helper()
	if err := apply(t, r, exts...); err != nil {
		t.Fatalf("ApplyExtensions: %v", err)
	}
}

func TestExtensionAddsFieldAndKeepsItsOwnText(t *testing.T) {
	r := hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"fields":[{"fieldname":"invoice_no","fieldtype":"Data","label":"Invoice no"}]}`))

	d, _ := r.Get("Lead")
	f := d.Field("invoice_no")
	if f == nil {
		t.Fatal("field was not added")
	}
	// the catalogue that owes the translation is the extending app's, not the host's
	if f.App != "billing" || d.FieldTextApp(f) != "billing" {
		t.Fatalf("field app=%q", f.App)
	}
	if d.FieldTextApp(d.Field("title")) != "crm" {
		t.Fatal("the host's own field should stay with the host")
	}
	if got := d.ExtendedBy; len(got) != 1 || got[0] != "billing" {
		t.Fatalf("ExtendedBy=%v", got)
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("merged meta must still validate: %v", err)
	}
}

func TestExtensionInsertsAfterAnExistingField(t *testing.T) {
	r := hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"fields":[{"fieldname":"invoice_no","fieldtype":"Data","insertAfter":"title"}]}`))
	d, _ := r.Get("Lead")
	if got := d.Fields[1].Fieldname; got != "invoice_no" {
		t.Fatalf("field order: %q at index 1", got)
	}

	r = hostRegistry()
	err := apply(t, r, ext("billing", "Lead", `{"fields":[{"fieldname":"x","fieldtype":"Data","insertAfter":"nope"}]}`))
	assertErr(t, err, `after "nope"`)
}

func TestExtensionSetsAProperty(t *testing.T) {
	r := hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"set":{"title":{"reqd":true,"label":"Company"}},"props":{"trackChanges":true}}`))

	d, _ := r.Get("Lead")
	f := d.Field("title")
	if !f.Reqd || f.Label != "Company" {
		t.Fatalf("title=%+v", f)
	}
	// overriding the label moves the key: billing wrote "Company", so billing translates it
	if d.FieldTextApp(f) != "billing" {
		t.Fatalf("label owner=%q", d.FieldTextApp(f))
	}
	if !d.TrackChanges {
		t.Fatal("trackChanges was not applied")
	}
	// a property nobody overrode keeps its value and its owner
	if d.Field("status").Label != "Status" || d.FieldTextApp(d.Field("status")) != "crm" {
		t.Fatal("untouched field changed")
	}
}

// nameLabel is a catalogue key, so overriding it moves the DocType's text to the
// extending app, as label does.
func TestExtensionSetsTheNameLabel(t *testing.T) {
	r := hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"props":{"nameLabel":"Lead No."}}`))

	d, _ := r.Get("Lead")
	if d.NameLabel != "Lead No." || d.TextAppOf() != "billing" {
		t.Fatalf("nameLabel=%q owner=%q", d.NameLabel, d.TextAppOf())
	}
}

func TestExtensionRefusesAPropertyThatIsIdentity(t *testing.T) {
	r := hostRegistry()
	err := apply(t, r,
		ext("billing", "Lead", `{"set":{"title":{"fieldtype":"Int"}}}`),
		ext("billing", "Role", `{"props":{"naming":{"field":"role_name"}}}`))
	assertErr(t, err, `may not override "fieldtype" on field "title"`)
	assertErr(t, err, `may not override "naming" on the DocType`)
}

// An extension cannot touch what migrate uses to move data: `renamedFrom` and
// `convert` determine DDL on the owner's column, and are not the extension's to declare.
// And what it overrides cannot wipe out what the owner declared.
func TestExtensionDoesNotTouchTheMigrationMachinery(t *testing.T) {
	r := hostRegistry()
	d, _ := r.Get("Lead")
	d.Field("title").RenamedFrom = Names{"nome"}
	d.Field("title").Convert = &Convert{From: "Int"}

	err := apply(t, r, ext("billing", "Lead", `{"set":{"title":{"renamedFrom":"outro","convert":{"from":"Data"}}}}`))
	assertErr(t, err, `may not override "renamedFrom" on field "title"`)
	assertErr(t, err, `may not override "convert" on field "title"`)

	// and a legitimate property setter on the same field preserves both
	r = hostRegistry()
	d, _ = r.Get("Lead")
	d.Field("title").RenamedFrom = Names{"nome"}
	d.Field("title").Convert = &Convert{From: "Int"}
	mustApply(t, r, ext("billing", "Lead", `{"set":{"title":{"reqd":true}}}`))
	f := d.Field("title")
	if !f.Reqd {
		t.Fatal("property setter was not applied")
	}
	if len(f.RenamedFrom) != 1 || f.RenamedFrom[0] != "nome" {
		t.Fatalf("renamedFrom lost in overlay: %v", f.RenamedFrom)
	}
	if f.Convert == nil || f.Convert.From != "Int" {
		t.Fatalf("convert lost in overlay: %+v", f.Convert)
	}
}

func TestExtensionRefusesToRetargetALink(t *testing.T) {
	r := hostRegistry()
	r.Add(&DocType{Name: "Deal", App: "crm", Fields: []*Field{
		{Fieldname: "lead", Fieldtype: "Link", Options: "Lead"},
		{Fieldname: "state", Fieldtype: "Select", Options: []any{"A"}},
	}})
	err := apply(t, r, ext("billing", "Deal", `{"set":{"lead":{"options":"Role"},"state":{"options":["A","B"]}}}`))
	assertErr(t, err, `may not override the options of "lead"`)

	// but a Select's options are text, and may be replaced
	r = hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"set":{"status":{"options":["Open","Won","Invoiced"]}}}`))
	d, _ := r.Get("Lead")
	if got := d.Field("status").SelectValues(); len(got) != 3 || got[2] != "Invoiced" {
		t.Fatalf("options=%v", got)
	}
}

func TestTwoAppsSettingTheSamePropertyRefuseToLoad(t *testing.T) {
	r := hostRegistry()
	a := apps()
	a.Order = append(a.Order, "support")
	a.Requires["support"] = []string{"crm"}
	err := r.ApplyExtensions([]*Extension{
		ext("billing", "Lead", `{"set":{"title":{"label":"Company"}}}`),
		ext("support", "Lead", `{"set":{"title":{"label":"Customer"}}}`),
	}, a)
	// whichever applies first, the other is the one that reports the clash
	assertErr(t, err, `already set by app "billing"`)
	assertErr(t, err, `also sets "label" on field "title"`)
}

func TestTwoAppsSettingDifferentPropertiesOfOneFieldIsFine(t *testing.T) {
	r := hostRegistry()
	a := apps()
	a.Order = append(a.Order, "support")
	a.Requires["support"] = []string{"crm"}
	if err := r.ApplyExtensions([]*Extension{
		ext("billing", "Lead", `{"set":{"title":{"reqd":true}}}`),
		ext("support", "Lead", `{"set":{"title":{"bold":true}}}`),
	}, a); err != nil {
		t.Fatal(err)
	}
	d, _ := r.Get("Lead")
	if f := d.Field("title"); !f.Reqd || !f.Bold {
		t.Fatalf("title=%+v", f)
	}
}

func TestExtensionCanOverrideFieldWidth(t *testing.T) {
	r := hostRegistry()
	if err := apply(t, r, ext("billing", "Lead", `{"set":{"title":{"width":"sm"}}}`)); err != nil {
		t.Fatal(err)
	}
	d, _ := r.Get("Lead")
	if f := d.Field("title"); f.Width != "sm" {
		t.Fatalf("title.Width=%q want sm", f.Width)
	}
}

func TestExtensionRefusesACollidingFieldname(t *testing.T) {
	r := hostRegistry()
	err := apply(t, r, ext("billing", "Lead", `{"fields":[{"fieldname":"title","fieldtype":"Data"}]}`))
	assertErr(t, err, `adds field "title", which app "crm" already defines`)

	// and the collision is caught between two extensions too
	r = hostRegistry()
	a := apps()
	a.Order = append(a.Order, "support")
	a.Requires["support"] = []string{"crm"}
	err = r.ApplyExtensions([]*Extension{
		ext("billing", "Lead", `{"fields":[{"fieldname":"ref","fieldtype":"Data"}]}`),
		ext("support", "Lead", `{"fields":[{"fieldname":"ref","fieldtype":"Data"}]}`),
	}, a)
	assertErr(t, err, `adds field "ref", which app "billing" already defines`)
}

func TestExtensionNeedsTheHostDeclaredAndMayNotExtendItself(t *testing.T) {
	r := hostRegistry()
	a := apps()
	a.Requires["billing"] = nil
	err := r.ApplyExtensions([]*Extension{ext("billing", "Lead", `{"fields":[{"fieldname":"x","fieldtype":"Data"}]}`)}, a)
	assertErr(t, err, `must declare requires: ["crm"]`)

	// core is always first, so it needs no declaration
	r = hostRegistry()
	if err := r.ApplyExtensions([]*Extension{ext("billing", "Role", `{"fields":[{"fieldname":"x","fieldtype":"Data"}]}`)}, a); err != nil {
		t.Fatalf("extending core should not need requires: %v", err)
	}

	r = hostRegistry()
	err = apply(t, r, ext("crm", "Lead", `{"fields":[{"fieldname":"x","fieldtype":"Data"}]}`))
	assertErr(t, err, "extends a DocType it owns")

	r = hostRegistry()
	err = apply(t, r, ext("billing", "Ghost", `{"fields":[{"fieldname":"x","fieldtype":"Data"}]}`))
	assertErr(t, err, "extends a DocType that does not exist")
}

func TestExtensionPermissionsOnlyAdd(t *testing.T) {
	r := hostRegistry()
	mustApply(t, r, ext("billing", "Lead", `{"permissions":[{"role":"Accountant","read":true}]}`))
	d, _ := r.Get("Lead")
	if len(d.Permissions) != 2 || d.Permissions[1].Role != "Accountant" {
		t.Fatalf("permissions=%+v", d.Permissions)
	}

	r = hostRegistry()
	err := apply(t, r, ext("billing", "Lead", `{"permissions":[{"role":"Sales","read":true,"delete":true}]}`))
	assertErr(t, err, `grants "Sales", which app "crm" already grants`)

	r = hostRegistry()
	err = apply(t, r, ext("billing", "Lead Note", `{"permissions":[{"role":"Accountant","read":true}]}`))
	assertErr(t, err, "child DocType")
}

func TestExtensionsApplyInAppOrder(t *testing.T) {
	r := hostRegistry()
	a := apps()
	a.Order = append(a.Order, "support")
	a.Requires["support"] = []string{"crm"}
	// declared support-first; the load order is what decides
	if err := r.ApplyExtensions([]*Extension{
		ext("support", "Lead", `{"fields":[{"fieldname":"ticket","fieldtype":"Data"}]}`),
		ext("billing", "Lead", `{"fields":[{"fieldname":"invoice_no","fieldtype":"Data"}]}`),
	}, a); err != nil {
		t.Fatal(err)
	}
	d, _ := r.Get("Lead")
	if got := d.Fields[len(d.Fields)-2].Fieldname; got != "invoice_no" {
		t.Fatalf("billing should come first: %q", got)
	}
	if got := d.ExtendedBy; len(got) != 2 || got[0] != "billing" || got[1] != "support" {
		t.Fatalf("ExtendedBy=%v", got)
	}
}

func TestExtensionReportsEveryProblemAtOnce(t *testing.T) {
	r := hostRegistry()
	err := apply(t, r, ext("billing", "Lead", `{"fields":[{"fieldname":"title","fieldtype":"Data"}],"set":{"nope":{"reqd":true}}}`))
	if err == nil {
		t.Fatal("expected errors")
	}
	if n := strings.Count(err.Error(), "\n  "); n != 2 {
		t.Fatalf("expected both problems listed, got:\n%v", err)
	}
}

func assertErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected an error mentioning %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not mention %q", err, want)
	}
}
