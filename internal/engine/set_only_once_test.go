package engine

import (
	"context"
	"strings"
	"testing"
)

var setOnceApp = map[string]string{
	"doctypes/tenant/tenant.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Tenant", module: "Demo", idGeneration: {field: "title"}, fields: [
 {fieldname: "title", fieldtype: "Data", label: "Title", reqd: true}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true}]});`,
	"doctypes/member_note/member_note.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Member Note", module: "Demo", isChild: true, fields: [
 {fieldname: "kind", fieldtype: "Data", label: "Kind", setOnlyOnce: true, inListView: true},
 {fieldname: "text", fieldtype: "Data", label: "Text", inListView: true}
]});`,
	"doctypes/member/member.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Member", module: "Demo", submittable: true, fields: [
 {fieldname: "tenant", fieldtype: "Link", label: "Tenant", options: "Tenant", setOnlyOnce: true},
 {fieldname: "nick", fieldtype: "Data", label: "Nick", allowOnSubmit: true},
 {fieldname: "notes", fieldtype: "Table", label: "Notes", options: "Member Note", allowOnSubmit: true}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true, submit: true, cancel: true}]});`,
	// a hook that moves the member when asked to: setOnlyOnce must catch it
	"doctypes/member/member.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Member", {
  beforeSave(doc) {
    if (doc.nick === "move") doc.tenant = "B";
  },
});`,
}

func wantOnceErr(t *testing.T, what string, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "cannot be changed once set") {
		t.Fatalf("%s: expected the setOnlyOnce refusal, got %v", what, err)
	}
}

func TestSetOnlyOnce(t *testing.T) {
	e := setupWith(t, setOnceApp)
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, n := range []string{"A", "B"} {
			if _, err := c.Insert(Doc{"doctype": "Tenant", "title": n}, SaveOpts{}); err != nil {
				return err
			}
		}
		m, err := c.Insert(Doc{"doctype": "Member", "tenant": "A", "nick": "x",
			"notes": []any{map[string]any{"kind": "k1", "text": "t"}}}, SaveOpts{})
		if err != nil {
			return err
		}
		id := m.ID()

		// save: the same value passes, a different one or a cleared one does not
		m["nick"] = "y"
		if m, err = c.Save(m, SaveOpts{}); err != nil {
			t.Fatalf("an untouched setOnlyOnce field blocked a save: %v", err)
		}
		m["tenant"] = "B"
		_, err = c.Save(m, SaveOpts{})
		wantOnceErr(t, "save changing it", err)
		m, _ = c.GetDoc("Member", id)
		m["tenant"] = nil
		_, err = c.Save(m, SaveOpts{})
		wantOnceErr(t, "save clearing it", err)
		// even with permissions ignored
		m, _ = c.GetDoc("Member", id)
		m["tenant"] = "B"
		_, err = c.Save(m, SaveOpts{IgnorePermissions: true})
		wantOnceErr(t, "save ignoring permissions", err)

		// a hook cannot slip a change past the check
		m, _ = c.GetDoc("Member", id)
		m["nick"] = "move"
		_, err = c.Save(m, SaveOpts{})
		wantOnceErr(t, "beforeSave changing it", err)

		// dbSet / setValue
		_, err = c.DBSet("Member", id, Doc{"tenant": "B"}, true)
		wantOnceErr(t, "dbSet changing it", err)
		err = c.SetValue("Member", id, Doc{"tenant": nil})
		wantOnceErr(t, "setValue clearing it", err)
		if _, err := c.DBSet("Member", id, Doc{"tenant": "A", "nick": "z"}, true); err != nil {
			t.Fatalf("dbSet with the same value: %v", err)
		}

		// a child row that exists keeps its value; a new row takes any
		m, _ = c.GetDoc("Member", id)
		rows := m.Children("notes")
		rows[0]["kind"] = "k2"
		m["notes"] = rows
		_, err = c.Save(m, SaveOpts{})
		wantOnceErr(t, "child row changing it", err)
		m, _ = c.GetDoc("Member", id)
		rowID := m.Children("notes")[0].ID()
		m["notes"] = append(m.Children("notes"), Doc{"kind": "k9", "text": "new"})
		if m, err = c.Save(m, SaveOpts{}); err != nil {
			t.Fatalf("a new child row: %v", err)
		}
		_, err = c.DBSet("Member Note", rowID, Doc{"kind": "k3"}, false)
		wantOnceErr(t, "dbSet on a child row", err)

		// submitted: an allowOnSubmit change passes, setOnlyOnce still holds
		m, _ = c.GetDoc("Member", id)
		if m, err = c.Submit(m); err != nil {
			return err
		}
		m["nick"] = "after"
		if m, err = c.Save(m, SaveOpts{}); err != nil {
			t.Fatalf("allowOnSubmit change: %v", err)
		}

		// an empty value is filled once, then holds
		empty, err := c.Insert(Doc{"doctype": "Member", "nick": "e"}, SaveOpts{})
		if err != nil {
			return err
		}
		empty["tenant"] = "A"
		if empty, err = c.Save(empty, SaveOpts{}); err != nil {
			t.Fatalf("filling an empty value: %v", err)
		}
		empty["tenant"] = "B"
		_, err = c.Save(empty, SaveOpts{})
		wantOnceErr(t, "changing a filled value", err)
		other, err := c.Insert(Doc{"doctype": "Member", "nick": "o"}, SaveOpts{})
		if err != nil {
			return err
		}
		if _, err := c.DBSet("Member", other.ID(), Doc{"tenant": "B"}, true); err != nil {
			t.Fatalf("dbSet filling an empty value: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// A Data Import update goes through Save, so a sheet cannot move a record
// either.
func TestSetOnlyOnceDataImport(t *testing.T) {
	e := setupWith(t, setOnceApp)
	ctx := context.Background()
	var id string
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		for _, n := range []string{"A", "B"} {
			if _, err := c.Insert(Doc{"doctype": "Tenant", "title": n}, SaveOpts{}); err != nil {
				return err
			}
		}
		m, err := c.Insert(Doc{"doctype": "Member", "tenant": "A", "nick": "x"}, SaveOpts{})
		id = m.ID()
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	res, err := runDataImport(t, e, "Admin", "en", DataImportArgs{Doctype: "Member", Mode: "update",
		File: []byte("id,Tenant\n" + id + ",B\n")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts.Errors != 1 || !strings.Contains(res.Rows[0].Message, "cannot be changed once set") {
		t.Fatalf("%+v\n%s", res.Counts, rowStatuses(res))
	}
}
