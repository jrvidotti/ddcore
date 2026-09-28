package engine

import (
	"context"
	"testing"
)

// computedLinkFiles plants a Track and a Track Note whose Link to it is
// computed: no column, so neither the delete check nor a rename may touch it.
// The note's Check field is where filters with 1/"1"/true are exercised.
var computedLinkFiles = map[string]string{
	"doctypes/track/track.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Track", allowRename: true, fields: [
  { fieldname: "title", fieldtype: "Data", label: "Title" } ] });`,
	"doctypes/track_note/track_note.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Track Note", fields: [
  { fieldname: "active", fieldtype: "Check", label: "Active" },
  { fieldname: "track", fieldtype: "Link", options: "Track", label: "Track", computed: true } ] });`,
}

func TestComputedLinkIgnoredOnDeleteAndRename(t *testing.T) {
	e := setupWith(t, computedLinkFiles)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		n, _ := c.NewDoc("Track Note", Doc{"active": 1})
		if _, err := c.Insert(n, SaveOpts{}); err != nil {
			return err
		}
		for _, id := range []string{"a", "b"} {
			tr, _ := c.NewDoc("Track", Doc{"id": id, "title": id})
			if _, err := c.Insert(tr, SaveOpts{}); err != nil {
				return err
			}
		}
		if _, err := c.Rename("Track", "a", "a2"); err != nil {
			t.Fatalf("rename with a computed Link pointing here: %v", err)
		}
		if err := c.Delete("Track", "b", false, false); err != nil {
			t.Fatalf("delete with a computed Link pointing here: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCheckFilterCoercesValue(t *testing.T) {
	e := setupWith(t, computedLinkFiles)
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		for _, v := range []any{1, 1, 0} {
			n, _ := c.NewDoc("Track Note", Doc{"active": v})
			if _, err := c.Insert(n, SaveOpts{}); err != nil {
				return err
			}
		}
		cases := []struct {
			filters any
			want    int64
		}{
			{map[string]any{"active": 1}, 2},
			{map[string]any{"active": float64(1)}, 2},
			{map[string]any{"active": "1"}, 2},
			{map[string]any{"active": true}, 2},
			{map[string]any{"active": 0}, 1},
			{map[string]any{"active": "0"}, 1},
			{[]any{[]any{"active", "!=", 1}}, 1},
			{[]any{[]any{"active", "in", []any{1}}}, 2},
			{[]any{[]any{"active", "in", "0,1"}}, 3},
			{[]any{[]any{"active", "not in", []any{"1"}}}, 1},
		}
		for _, tc := range cases {
			got, err := c.Count("Track Note", tc.filters)
			if err != nil {
				t.Fatalf("Count(%v): %v", tc.filters, err)
			}
			if got != tc.want {
				t.Fatalf("Count(%v) = %d, want %d", tc.filters, got, tc.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
