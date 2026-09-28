package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExtensionWritesTheExtendFile(t *testing.T) {
	dir := t.TempDir()
	files, err := Extension(dir, "Sales Lead", ExtensionSpec{
		Fields:      []map[string]any{{"fieldname": "invoice_no", "fieldtype": "Data", "label": "Invoice no", "insertAfter": "title"}},
		Set:         map[string]map[string]any{"status": {"reqd": true}},
		Doctype:     map[string]any{"trackChanges": true},
		Permissions: []map[string]any{{"role": "Accountant", "read": true}},
		WithForm:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(dir, "extensions/sales_lead.extend.ts"), filepath.Join(dir, "extensions/sales_lead.form.ts")}
	if len(files) != 2 || files[0] != want[0] || files[1] != want[1] {
		t.Fatalf("files = %v, want %v", files, want)
	}
	b, _ := os.ReadFile(files[0])
	src := string(b)
	t.Log("\n" + src)
	for _, s := range []string{`extendDoctype("Sales Lead", {`, `insertAfter: "title"`, `status: {`, `trackChanges: true`, `role: "Accountant"`, "// hasPermission"} {
		if !strings.Contains(src, s) {
			t.Errorf("source lacks %q", s)
		}
	}
	// sections in the order the docs use
	if i, j := strings.Index(src, "fields:"), strings.Index(src, "permissions:"); i < 0 || j < i {
		t.Errorf("fields should come before permissions")
	}
	f, _ := os.ReadFile(files[1])
	if !strings.Contains(string(f), `defineForm("Sales Lead"`) {
		t.Errorf("form script: %s", f)
	}
}

func TestExtensionOnlyCreates(t *testing.T) {
	dir := t.TempDir()
	spec := ExtensionSpec{Set: map[string]map[string]any{"status": {"reqd": true}}}
	if _, err := Extension(dir, "Lead", spec); err != nil {
		t.Fatal(err)
	}
	if _, err := Extension(dir, "Lead", spec); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second call: %v", err)
	}
}

func TestExtensionRefusesAnEmptySpec(t *testing.T) {
	dir := t.TempDir()
	if _, err := Extension(dir, "Lead", ExtensionSpec{WithForm: true}); err == nil {
		t.Fatal("an extension with nothing in it should be refused")
	}
	if _, err := os.Stat(filepath.Join(dir, "extensions")); err == nil {
		t.Fatal("nothing should be written")
	}
}
