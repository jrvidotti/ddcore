package engine

import (
	"testing"
)

func TestVersionDocType_ListViewAndFilterFields(t *testing.T) {
	e := setup(t)
	d, err := e.DocType("Version")
	if err != nil {
		t.Fatalf("load Version DocType: %v", err)
	}

	if d.SortField != "creation" || d.SortOrder != "desc" {
		t.Errorf("expected SortField=creation, SortOrder=desc; got %s %s", d.SortField, d.SortOrder)
	}

	for _, fieldname := range []string{"ref_doctype", "docname"} {
		f := d.Field(fieldname)
		if f == nil {
			t.Fatalf("field %s missing from Version DocType", fieldname)
		}
		if !f.InListView {
			t.Errorf("expected field %s to have InListView == true", fieldname)
		}
		if !f.InStandardFilter {
			t.Errorf("expected field %s to have InStandardFilter == true", fieldname)
		}
	}
}
