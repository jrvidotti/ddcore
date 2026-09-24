package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

var multiSelectApp = map[string]string{
	"doctypes/tag/tag.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Tag", module: "Demo", idGeneration: { field: "title" }, fields: [
 {fieldname: "title", fieldtype: "Data", label: "Title", reqd: true}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true}]});`,
	"doctypes/note_tag/note_tag.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Note Tag", module: "Demo", isChild: true, fields: [
 {fieldname: "tag", fieldtype: "Link", label: "Tag", options: "Tag", reqd: true}
]});`,
	"doctypes/note_row/note_row.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Note Row", module: "Demo", isChild: true, fields: [
 {fieldname: "tag", fieldtype: "Link", label: "Tag", options: "Tag", reqd: true}
]});`,
	"doctypes/note/note.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({name: "Note", module: "Demo", trackChanges: true, submittable: true, fields: [
 {fieldname: "title", fieldtype: "Data", label: "Title"},
 {fieldname: "tags", fieldtype: "Table MultiSelect", label: "Tags", options: "Note Tag"},
 {fieldname: "required_tags", fieldtype: "Table MultiSelect", label: "Required tags", options: "Note Row", reqd: true}
], permissions: [{role: "All", read: true, write: true, create: true, delete: true, submit: true, cancel: true}]});`,
}

func tagValues(d Doc, field string) []string {
	var out []string
	for _, r := range d.Children(field) {
		out = append(out, r.Str("tag"))
	}
	return out
}

func seedTags(t *testing.T, c *Ctx, names ...string) {
	t.Helper()
	for _, n := range names {
		doc, err := c.NewDoc("Tag", Doc{"title": n})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.Insert(doc, SaveOpts{}); err != nil {
			t.Fatal(err)
		}
	}
}

// The value is rows, like a Table's, whichever shape it was written in: a
// list of ids is the convenient input, and rows are what comes back.
func TestTableMultiSelectRoundTrip(t *testing.T) {
	e := setupWith(t, multiSelectApp)
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		seedTags(t, c, "red", "green", "blue")
		doc, err := c.NewDoc("Note", Doc{"title": "One", "tags": []any{"red", "green"}, "required_tags": []string{"blue"}})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		loaded, err := c.GetDoc("Note", saved.ID())
		if err != nil {
			return err
		}
		if got := strings.Join(tagValues(loaded, "tags"), ","); got != "red,green" {
			t.Fatalf("tags came back as %q (%#v)", got, loaded["tags"])
		}
		rows := loaded.Children("tags")
		if rows[0].Str("id") == "" || rows[1]["idx"] != int64(2) || rows[0].Str("parentfield") != "tags" {
			t.Fatalf("rows are not child rows: %#v", rows)
		}
		firstID := rows[0].Str("id")

		// an untouched save, and the same ids sent again, change nothing
		if _, err := c.Save(loaded, SaveOpts{}); err != nil {
			return err
		}
		again, _ := c.GetDoc("Note", saved.ID())
		again["tags"] = []any{"red", "green"}
		if _, err := c.Save(again, SaveOpts{}); err != nil {
			return err
		}
		var versions int
		if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Note'`).Scan(&versions); err != nil {
			return err
		}
		if versions != 0 {
			t.Fatalf("re-sending the same values wrote %d version(s)", versions)
		}
		kept, _ := c.GetDoc("Note", saved.ID())
		if kept.Children("tags")[0].Str("id") != firstID {
			t.Fatal("a value that stayed lost its row")
		}

		// removing and reordering is a change, recorded as one
		kept["tags"] = []any{"blue", "red"}
		if _, err := c.Save(kept, SaveOpts{}); err != nil {
			return err
		}
		after, _ := c.GetDoc("Note", saved.ID())
		if got := strings.Join(tagValues(after, "tags"), ","); got != "blue,red" {
			t.Fatalf("tags after the change: %q", got)
		}
		if after.Children("tags")[1].Str("id") != firstID {
			t.Fatal("red moved, and should have kept its row")
		}
		var rowsLeft int
		if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_note_tag WHERE parent = $1`, saved.ID()).Scan(&rowsLeft); err != nil {
			return err
		}
		if rowsLeft != 2 {
			t.Fatalf("the removed value left its row behind: %d rows", rowsLeft)
		}
		if err := c.Q().QueryRow(c.Ctx, `SELECT count(*) FROM tab_version WHERE ref_doctype = 'Note'`).Scan(&versions); err != nil {
			return err
		}
		if versions != 1 {
			t.Fatalf("the change wrote %d version(s)", versions)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestTableMultiSelectRefusals(t *testing.T) {
	e := setupWith(t, multiSelectApp)
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		seedTags(t, c, "red")
		for name, tc := range map[string]struct {
			doc  Doc
			want string
		}{
			"duplicate": {Doc{"tags": []any{"red", "red"}, "required_tags": []any{"red"}}, "more than once"},
			"missing":   {Doc{"tags": []any{"nope"}, "required_tags": []any{"red"}}, "nope"},
			"empty row": {Doc{"tags": []any{map[string]any{}}, "required_tags": []any{"red"}}, "Tag"},
			"reqd":      {Doc{"tags": []any{"red"}, "required_tags": []any{}}, "Required tags"},
		} {
			doc, err := c.NewDoc("Note", tc.doc)
			if err != nil {
				return err
			}
			_, err = c.Insert(doc, SaveOpts{})
			if err == nil {
				t.Errorf("%s: saved", name)
				continue
			}
			if msg := cerr.From(err).Message; !strings.Contains(msg, tc.want) {
				t.Errorf("%s: wanted %q in %q", name, tc.want, msg)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// A submitted document's values are as fixed as a Table's rows, and sending
// the same ids again is not a change.
func TestTableMultiSelectAfterSubmit(t *testing.T) {
	e := setupWith(t, multiSelectApp)
	if err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		seedTags(t, c, "red", "green")
		doc, err := c.NewDoc("Note", Doc{"tags": []any{"red"}, "required_tags": []any{"green"}, "docstatus": 1})
		if err != nil {
			return err
		}
		saved, err := c.Insert(doc, SaveOpts{})
		if err != nil {
			return err
		}
		same, _ := c.GetDoc("Note", saved.ID())
		same["tags"] = []any{"red"}
		if _, err := c.Save(same, SaveOpts{}); err != nil {
			t.Fatalf("the same values after submit: %v", err)
		}
		changed, _ := c.GetDoc("Note", saved.ID())
		changed["tags"] = []any{"red", "green"}
		if _, err := c.Save(changed, SaveOpts{}); err == nil {
			t.Fatal("a submitted document's values changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// Each value is a Link, so a scoped user cannot choose one outside the scope.
func TestTableMultiSelectRespectsScopes(t *testing.T) {
	e := setupWith(t, multiSelectApp)
	const user = "tagger@x.com"
	ctx := context.Background()
	if err := e.Run(ctx, "Admin", func(c *Ctx) error {
		seedTags(t, c, "red", "blue")
		u, err := c.NewDoc("User", Doc{"email": user, "full_name": "Tagger"})
		if err != nil {
			return err
		}
		if _, err := c.Insert(u, SaveOpts{}); err != nil {
			return err
		}
		p, err := c.NewDoc("User Permission", Doc{"user": user, "allow": "Tag", "for_value": "red"})
		if err != nil {
			return err
		}
		_, err = c.Insert(p, SaveOpts{})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	err := e.Run(ctx, user, func(c *Ctx) error {
		doc, err := c.NewDoc("Note", Doc{"tags": []any{"red", "blue"}, "required_tags": []any{"red"}})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{})
		return err
	})
	if err == nil {
		t.Fatal("a scoped user chose a value outside the scope")
	}
	if err := e.Run(ctx, user, func(c *Ctx) error {
		doc, err := c.NewDoc("Note", Doc{"tags": []any{"red"}, "required_tags": []any{"red"}})
		if err != nil {
			return err
		}
		_, err = c.Insert(doc, SaveOpts{})
		return err
	}); err != nil {
		t.Fatalf("a value inside the scope: %v", err)
	}
}
