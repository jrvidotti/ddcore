package meta

import (
	"strings"
	"testing"
)

func linkTitleRegistry(link *Field) *Registry {
	r := NewRegistry()
	r.Add(&DocType{Name: "Category", TitleField: "full_name", Fields: []*Field{
		{Fieldname: "category_name", Fieldtype: "Data"},
		{Fieldname: "full_name", Fieldtype: "Data"},
		{Fieldname: "short_name", Fieldtype: "Data"},
		{Fieldname: "sign", Fieldtype: "Signature"},
		{Fieldname: "pwd", Fieldtype: "Password"},
		{Fieldname: "secret", Fieldtype: "Data", Permlevel: 1},
		{Fieldname: "derived", Fieldtype: "Data", Computed: true},
	}, Permissions: []Perm{{Role: "System Manager", Read: true}, {Role: "System Manager", Read: true, Permlevel: 1}}})
	r.Add(&DocType{Name: "Status", TranslateID: true, Fields: []*Field{{Fieldname: "label", Fieldtype: "Data"}}})
	r.Add(&DocType{Name: "Plan", Fields: []*Field{link}})
	return r
}

func TestLinkTitleFieldValidation(t *testing.T) {
	for _, f := range []*Field{
		{Fieldname: "category", Fieldtype: "Link", Options: "Category", LinkTitleField: "short_name"},
		{Fieldname: "category", Fieldtype: "Link", Options: "Category", LinkTitleField: "id"},
		{Fieldname: "category", Fieldtype: "Link", Options: "Category", LinkTitleField: "modified_by"},
	} {
		if err := linkTitleRegistry(f).Validate(); err != nil {
			t.Errorf("%s: %v", f.LinkTitleField, err)
		}
	}
	for _, c := range []struct {
		f    *Field
		want string
	}{
		{&Field{Fieldname: "c", Fieldtype: "Data", LinkTitleField: "short_name"}, "linkTitleField is for a Link field, not a Data"},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Category", LinkTitleField: "nope"}, `linkTitleField "nope" is not a field of "Category"`},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Category", LinkTitleField: "sign"}, "is a Signature, whose value is too large"},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Category", LinkTitleField: "pwd"}, "is a Password, which holds a secret"},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Category", LinkTitleField: "secret"}, "has permlevel 1; a title must be permlevel 0"},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Category", LinkTitleField: "derived"}, "has no column to read"},
		{&Field{Fieldname: "c", Fieldtype: "Link", Options: "Status", LinkTitleField: "label"}, "whose title is its translated id"},
	} {
		err := linkTitleRegistry(c.f).Validate()
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%+v: want %q, got %v", c.f, c.want, err)
		}
	}
}

func TestDeclaredLinkTitle(t *testing.T) {
	r := linkTitleRegistry(&Field{Fieldname: "category", Fieldtype: "Link", Options: "Category", LinkTitleField: "short_name"})
	if !r.DeclaredLinkTitle("Category", "short_name") {
		t.Fatal("the pair a Link declares must be declared")
	}
	if r.DeclaredLinkTitle("Category", "category_name") || r.DeclaredLinkTitle("Plan", "short_name") {
		t.Fatal("a pair no Link declares must not be")
	}
}
