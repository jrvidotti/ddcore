package engine

import (
	"context"
	"encoding/json"
	"testing"
)

// The fixture for #28: a tracked DocType with a child table and a secret on
// both levels, a controller that refuses one delete, and an untracked DocType.
var deletionFixture = map[string]string{
	"doctypes/memo_line/memo_line.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Memo Line", isChild: true, fields: [
  { fieldname: "text", fieldtype: "Data", label: "Text" },
  { fieldname: "line_pin", fieldtype: "Password", label: "Line PIN" },
]});`,
	"doctypes/memo/memo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Memo", trackChanges: true, idGeneration: { field: "title" }, fields: [
  { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
  { fieldname: "body", fieldtype: "Data", label: "Body" },
  { fieldname: "pin", fieldtype: "Password", label: "PIN" },
  { fieldname: "lines", fieldtype: "Table", label: "Lines", options: "Memo Line" },
], permissions: [{ role: "All", read: true, write: true, create: true, delete: true }] });`,
	"doctypes/memo/memo.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Memo", {
  onTrash(doc) { if (doc.body === "keep") ddcore.throw("kept"); },
});`,
	"doctypes/scrap/scrap.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Scrap", fields: [
  { fieldname: "label", fieldtype: "Data", label: "Label" },
], permissions: [{ role: "All", read: true, write: true, create: true, delete: true }] });`,
}

func versionsOf(t *testing.T, e *Engine, doctype, id string) []map[string]any {
	t.Helper()
	return sqlRows(t, e, `SELECT id, deleted, data FROM tab_version WHERE ref_doctype = $1 AND doc_id = $2 ORDER BY creation`, doctype, id)
}

// jsonMap reads a JSON column whichever way the driver handed it over.
func jsonMap(t *testing.T, v any) map[string]any {
	t.Helper()
	if m, ok := v.(map[string]any); ok {
		return m
	}
	var m map[string]any
	if s, ok := v.(string); ok && json.Unmarshal([]byte(s), &m) == nil {
		return m
	}
	t.Fatalf("not a JSON object: %#v", v)
	return nil
}

func deleteAudits(t *testing.T, e *Engine, doctype, id string) []map[string]any {
	t.Helper()
	return sqlRows(t, e, `SELECT actor, detail FROM tab_audit_event WHERE action = 'doc.delete' AND target_doctype = $1 AND target_id = $2`, doctype, id)
}

// TestDeleteKeepsHistoryAndRecordsTheDocument — #28: a delete erased the
// document's Versions and left no trace of who deleted what.
func TestDeleteKeepsHistoryAndRecordsTheDocument(t *testing.T) {
	e := setupWith(t, deletionFixture)
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, _ := c.NewDoc("Memo", Doc{"title": "m1", "body": "a", "pin": "1234",
			"lines": []any{map[string]any{"text": "first", "line_pin": "9999"}}})
		doc, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		for _, body := range []string{"b", "c"} {
			doc["body"] = body
			if doc, err = c.Save(doc, SaveOpts{}); err != nil {
				return err
			}
		}
		return c.Delete("Memo", "m1", false, false)
	})
	if err != nil {
		t.Fatal(err)
	}

	vs := versionsOf(t, e, "Memo", "m1")
	if len(vs) != 3 {
		t.Fatalf("want the 2 saves and the deletion, got %d Versions", len(vs))
	}
	if vs[0]["deleted"] != false || vs[1]["deleted"] != false || vs[2]["deleted"] != true {
		t.Fatalf("only the last Version is the deletion: %v", vs)
	}
	snap, _ := jsonMap(t, vs[2]["data"])["deleted"].(map[string]any)
	if snap["body"] != "c" || snap["title"] != "m1" {
		t.Fatalf("the snapshot is not the document as it was: %v", snap)
	}
	if _, ok := snap["pin"]; ok {
		t.Fatal("a Password field reached the snapshot")
	}
	lines, _ := snap["lines"].([]any)
	if len(lines) != 1 {
		t.Fatalf("the child rows are missing from the snapshot: %v", snap["lines"])
	}
	line := lines[0].(map[string]any)
	if line["text"] != "first" {
		t.Fatalf("the child row is not as it was: %v", line)
	}
	if _, ok := line["line_pin"]; ok {
		t.Fatal("a child's Password field reached the snapshot")
	}

	audits := deleteAudits(t, e, "Memo", "m1")
	if len(audits) != 1 || audits[0]["actor"] != "Admin" {
		t.Fatalf("want one doc.delete by Admin, got %v", audits)
	}
	detail := jsonMap(t, audits[0]["detail"])
	if detail["version"] != vs[2]["id"] {
		t.Fatalf("the audit does not point at the deletion Version: %v", detail)
	}
}

// A refused delete rolls back with the transaction: no deletion Version, no
// audit, and the history the document already had is still there.
func TestRefusedDeleteLeavesNoRecord(t *testing.T) {
	e := setupWith(t, deletionFixture)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, _ := c.NewDoc("Memo", Doc{"title": "m2", "body": "a"})
		doc, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		doc["body"] = "keep"
		_, err = c.Save(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := e.Run(ctx, "Admin", func(c *Ctx) error { return c.Delete("Memo", "m2", false, false) })
	if err == nil {
		t.Fatal("onTrash should have refused the delete")
	}
	vs := versionsOf(t, e, "Memo", "m2")
	if len(vs) != 1 || vs[0]["deleted"] != false {
		t.Fatalf("want the one save and no deletion, got %v", vs)
	}
	if a := deleteAudits(t, e, "Memo", "m2"); len(a) != 0 {
		t.Fatalf("a refused delete was audited: %v", a)
	}
}

// Every DocType leaves a deletion Version, tracked or not; deleting a Version
// leaves none — only the audit says it happened.
func TestDeletionRecordedForEveryDocTypeButVersion(t *testing.T) {
	e := setupWith(t, deletionFixture)
	ctx := context.Background()
	var scrap string
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		doc, _ := c.NewDoc("Scrap", Doc{"label": "x"})
		doc, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		scrap = doc.ID()
		return c.Delete("Scrap", scrap, false, false)
	}); err != nil {
		t.Fatal(err)
	}
	vs := versionsOf(t, e, "Scrap", scrap)
	if len(vs) != 1 || vs[0]["deleted"] != true {
		t.Fatalf("an untracked DocType leaves one deletion Version, got %v", vs)
	}
	versionID := vs[0]["id"].(string)
	if err := e.Run(ctx, "Admin", func(c *Ctx) error { return c.Delete("Version", versionID, false, false) }); err != nil {
		t.Fatal(err)
	}
	if n := sqlRows(t, e, `SELECT 1 FROM tab_version WHERE doc_id = $1`, versionID); len(n) != 0 {
		t.Fatal("deleting a Version wrote a Version of it")
	}
	if a := deleteAudits(t, e, "Version", versionID); len(a) != 1 {
		t.Fatalf("deleting a Version is still audited, got %v", a)
	}
}

// A document created again under a deleted one's id is another document: a
// reader of the new one does not get the old one's history. A System Manager
// still reads all of it.
func TestReusedIDDoesNotInheritHistory(t *testing.T) {
	e := setupWith(t, deletionFixture)
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		c.Flags["ignorePermissions"] = true
		u, _ := c.NewDoc("User", Doc{"email": "reader@x.com", "full_name": "Reader"})
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		c.Flags["ignorePermissions"] = false
		doc, _ := c.NewDoc("Memo", Doc{"title": "m3", "body": "old"})
		doc, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		doc["body"] = "older"
		if _, err := c.Save(doc, SaveOpts{}); err != nil {
			return err
		}
		if err := c.Delete("Memo", "m3", false, false); err != nil {
			return err
		}
		again, _ := c.NewDoc("Memo", Doc{"title": "m3", "body": "new"})
		again, err = c.Insert(again, SaveOpts{})
		if err != nil {
			return err
		}
		again["body"] = "newer"
		_, err = c.Save(again, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	all := versionsOf(t, e, "Memo", "m3")
	if len(all) != 3 {
		t.Fatalf("want old save, deletion, new save; got %v", all)
	}
	filters := map[string]any{"ref_doctype": "Memo", "doc_id": "m3"}
	if err := e.Run(ctx, "reader@x.com", func(c *Ctx) error {
		rows, err := c.GetList("Version", ListArgs{Filters: filters, Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 1 || rows[0]["id"] != all[2]["id"] {
			t.Fatalf("the reader sees the deleted document's history: %v", rows)
		}
		if n, err := c.Count("Version", filters); err != nil || n != 1 {
			t.Fatalf("the count disagrees with the list: %d, %v", n, err)
		}
		if _, err := c.GetDoc("Version", all[0]["id"].(string)); err == nil {
			t.Fatal("the reader opened a Version of the deleted document")
		}
		if _, err := c.GetDoc("Version", all[2]["id"].(string)); err != nil {
			t.Fatalf("the reader cannot open the new document's own Version: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		rows, err := c.GetList("Version", ListArgs{Filters: filters, Fields: []string{"id"}})
		if err != nil {
			return err
		}
		if len(rows) != 3 {
			t.Fatalf("a System Manager reads the whole history, got %v", rows)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
