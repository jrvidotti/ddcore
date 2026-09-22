package engine

import "testing"

const sampleMap = `{
  "ignoreUnknown": true,
  "users": {"velho@x.com": "novo@x.com"},
  "exclude": ["Error Log"],
  "doctypes": {
    "Nota Fiscal": {
      "to": "Invoice",
      "fields": {"cliente": "customer", "legado_x": null},
      "set": {"company": "Acme"},
      "values": {"situacao": {"Rascunho": "Draft"}},
      "ids": {"NF-1": "INV-1"},
      "onExisting": "error",
      "children": {"itens": {"to": "lines", "fields": {"qtd": "qty"}}}
    }
  }
}`

func TestImportMapAppliesEveryRule(t *testing.T) {
	m, err := ParseImportMap([]byte(sampleMap))
	if err != nil {
		t.Fatal(err)
	}
	doc := Doc{
		"doctype": "Nota Fiscal", "id": "NF-1", "cliente": "ACME", "legado_x": 7,
		"situacao": "Rascunho", "owner": "velho@x.com", "modified_by": "outro@x.com",
		"itens": []any{map[string]any{"doctype": "Item", "id": "i1", "qtd": 2}},
	}
	dt, out := m.Apply("Nota Fiscal", doc)
	if dt != "Invoice" {
		t.Fatalf("doctype = %q", dt)
	}
	if out["customer"] != "ACME" {
		t.Fatalf("field rename lost: %v", out)
	}
	if _, ok := out["legado_x"]; ok {
		t.Fatalf("dropped field survived: %v", out)
	}
	if out["company"] != "Acme" {
		t.Fatalf("constant not set: %v", out)
	}
	if out["situacao"] != "Draft" {
		t.Fatalf("value not remapped: %v", out)
	}
	if out["id"] != "INV-1" {
		t.Fatalf("id not remapped: %v", out)
	}
	if out["owner"] != "novo@x.com" || out["modified_by"] != "outro@x.com" {
		t.Fatalf("owner=%v modified_by=%v", out["owner"], out["modified_by"])
	}
	rows, ok := out["lines"].([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("children = %#v", out["lines"])
	}
	row := rows[0].(map[string]any)
	if row["qty"] != 2 {
		t.Fatalf("child field rename lost: %v", row)
	}
	if _, ok := out["itens"]; ok {
		t.Fatalf("the old table fieldname survived: %v", out)
	}
}

// Applying must not touch the document the reader handed over: a failed record
// is retried in careful mode from the same line.
func TestImportMapLeavesTheSourceDocumentAlone(t *testing.T) {
	m, _ := ParseImportMap([]byte(sampleMap))
	doc := Doc{"doctype": "Nota Fiscal", "id": "NF-1", "cliente": "ACME"}
	m.Apply("Nota Fiscal", doc)
	if doc["cliente"] != "ACME" || doc["id"] != "NF-1" {
		t.Fatalf("source mutated: %v", doc)
	}
}

func TestImportMapRemapsIDsForLinkTargets(t *testing.T) {
	m, _ := ParseImportMap([]byte(sampleMap))
	if got := m.RemapID("Nota Fiscal", "NF-1"); got != "INV-1" {
		t.Fatalf("RemapID = %q", got)
	}
	if got := m.RemapID("Nota Fiscal", "NF-9"); got != "NF-9" {
		t.Fatalf("an unmapped id must pass through, got %q", got)
	}
	if got := m.RemapUser("velho@x.com"); got != "novo@x.com" {
		t.Fatalf("RemapUser = %q", got)
	}
}

func TestImportMapTargetsAndExclusions(t *testing.T) {
	m, _ := ParseImportMap([]byte(sampleMap))
	if got := m.Target("Nota Fiscal"); got != "Invoice" {
		t.Fatalf("Target = %q", got)
	}
	if got := m.Target("Task"); got != "Task" {
		t.Fatalf("an unmapped doctype keeps its name, got %q", got)
	}
	if !m.Excluded("Error Log") || m.Excluded("Task") {
		t.Fatal("exclusions wrong")
	}
	if got := m.OnExisting("Nota Fiscal"); got != OnExistingError {
		t.Fatalf("OnExisting = %q", got)
	}
	if got := m.OnExisting("Task"); got != OnExistingIdentical {
		t.Fatalf("default OnExisting = %q", got)
	}
}

func TestParseImportMapRefusesAnUnknownPolicy(t *testing.T) {
	_, err := ParseImportMap([]byte(`{"doctypes":{"Task":{"onExisting":"overwrite"}}}`))
	if err == nil {
		t.Fatal("an unknown onExisting must be refused, not ignored")
	}
}

// No mapping file is the ordinary case: a nil map applies nothing.
func TestNilImportMapIsIdentity(t *testing.T) {
	var m *ImportMap
	dt, out := m.Apply("Task", Doc{"id": "T-1"})
	if dt != "Task" || out["id"] != "T-1" {
		t.Fatalf("dt=%q out=%v", dt, out)
	}
	if m.Excluded("Task") || m.OnExisting("Task") != OnExistingIdentical {
		t.Fatal("nil map should answer the defaults")
	}
}
