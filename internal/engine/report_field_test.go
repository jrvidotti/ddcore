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
