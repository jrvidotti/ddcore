package engine

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestCheckReportFields(t *testing.T) {
	reg := meta.NewRegistry()
	reg.Add(&meta.DocType{Name: "Course", Fields: []*meta.Field{
		{Fieldname: "attendance", Fieldtype: "Report", Options: "Course Attendance"},
		{Fieldname: "grades", Fieldtype: "Report", Options: "Nope"},
	}})
	err := checkReportFields(reg, map[string]map[string]any{"Course Attendance": {}})
	if err == nil || !strings.Contains(err.Error(), `field "grades" points at report "Nope"`) || strings.Contains(err.Error(), "attendance") {
		t.Fatalf("err = %v", err)
	}
	if err := checkReportFields(reg, map[string]map[string]any{"Course Attendance": {}, "Nope": {}}); err != nil {
		t.Fatal(err)
	}
}

func TestCheckReportFilters(t *testing.T) {
	ok := map[string]map[string]any{"Sales": {"filters": []any{
		map[string]any{"fieldname": "year"},
		map[string]any{"fieldname": "group_by", "placement": "table"},
		map[string]any{"fieldname": "show", "description": "How each cell adds up", "descriptionPlacement": "tooltip"},
		map[string]any{"fieldname": "notes", "descriptionPlacement": "below"},
	}}}
	if err := checkReportFilters(ok); err != nil {
		t.Fatal(err)
	}
	bad := map[string]map[string]any{"Sales": {"filters": []any{map[string]any{"fieldname": "top", "placement": "grid"}}}}
	err := checkReportFilters(bad)
	if err == nil || !strings.Contains(err.Error(), `report "Sales": filter "top" has placement grid`) {
		t.Fatalf("err = %v", err)
	}
	tip := map[string]map[string]any{"Sales": {"filters": []any{map[string]any{"fieldname": "show", "descriptionPlacement": "hover"}}}}
	err = checkReportFilters(tip)
	if err == nil || !strings.Contains(err.Error(), `report "Sales": filter "show" has descriptionPlacement hover; it must be "below" or "tooltip"`) {
		t.Fatalf("err = %v", err)
	}
}
