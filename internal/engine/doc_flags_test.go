package engine

import (
	"context"
	"strings"
	"testing"
)

// docFlagsApp plants two DocTypes whose hooks record what they see in
// doc.flags: Thing refuses a write that is not flagged as the system's, and
// its onUpdate inserts an Echo, whose own hooks must not inherit the flags.
func docFlagsApp() map[string]string {
	return map[string]string{
		"doctypes/thing/thing.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Thing", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "note", fieldtype: "Data", label: "Note" },
    { fieldname: "seen", fieldtype: "Data", label: "Seen" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/thing/thing.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Thing", {
  validate(doc) {
    if (doc.note === "locked" && !doc.flags.system) ddcore.throw("refused");
    const before = doc.getDocBeforeSave();
    if (before && before.note === "forged") ddcore.throw("forged before");
    doc.flags.fromValidate = "v";
  },
  onUpdate(doc) {
    // what validate left has to be here, whoever started the save
    doc.dbSet("seen", String(doc.flags.fromValidate));
    doc.flags.fromUpdate = "u";
    if (doc.flags.echo) ddcore.newDoc("Echo", { title: doc.title }).insert();
  },
  onTrash(doc) {
    if (!doc.flags.system) ddcore.throw("refused delete");
  },
});`,
		"doctypes/echo/echo.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Echo", idGeneration: { field: "title" },
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true },
    { fieldname: "inherited", fieldtype: "Data", label: "Inherited" },
  ],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, delete: true }] });`,
		"doctypes/echo/echo.controller.ts": `import { defineController } from "@ddcore/sdk";
export default defineController("Echo", {
  validate(doc) { doc.inherited = Object.keys(doc.flags).filter((k) => k !== "seen").join(","); },
});`,
	}
}

// #55: a flag set on the document before save() or insert() is what the hooks
// of that write see, and what they leave is back on the caller's document.
func TestDocFlagsReachTheHooksAndComeBack(t *testing.T) {
	e := setupWith(t, docFlagsApp())
	out, _, err := e.Eval(context.Background(), `
const refused = (fn: () => void) => { try { fn(); return false; } catch (e) { return true; } };
const r: any = {};

const a = ddcore.newDoc("Thing", { title: "A", note: "locked" });
r.insertRefused = refused(() => ddcore.newDoc("Thing", { title: "A", note: "locked" }).insert());
a.flags.system = true;
a.insert();
r.afterInsert = [a.flags.system, a.flags.fromValidate, a.flags.fromUpdate];

const b = ddcore.getDoc("Thing", "A");
b.note = "locked";
r.saveRefused = refused(() => { const x = ddcore.getDoc("Thing", "A"); x.title = "A"; x.note = "locked"; x.save(); });
const held = b.flags;
b.flags.system = true;
b.save();
r.afterSave = [b.flags.fromValidate, b.flags.fromUpdate, held === b.flags];
r.seen = ddcore.db.getValue("Thing", "A", "seen");

const c = ddcore.getDoc("Thing", "A");
c.flags.system = true;
c.flags.echo = true;
c.save();
r.echo = [!!ddcore.db.exists("Echo", "A"), ddcore.db.getValue("Echo", "A", "inherited") || ""];

r.deleteRefused = refused(() => ddcore.getDoc("Thing", "A").delete());
const d = ddcore.getDoc("Thing", "A");
d.flags.system = true;
d.delete();
r.deleted = !ddcore.db.exists("Thing", "A");
JSON.stringify(r);`, false)
	if err != nil {
		t.Fatal(err)
	}
	want := `"{\"insertRefused\":true,\"afterInsert\":[true,\"v\",\"u\"],\"saveRefused\":true,\"afterSave\":[\"v\",\"u\",true],\"seen\":\"v\",\"echo\":[true,\"\"],\"deleteRefused\":true,\"deleted\":true}"`
	if string(out) != want {
		t.Fatalf("doc.flags did not travel with the write:\n got %s\nwant %s", out, want)
	}
}

// A write that starts in Go — the REST API, the desk — has no caller's flags,
// but its hooks still share one object: validate's note reaches onUpdate.
func TestDocFlagsAreSharedByTheHooksOfAGoWrite(t *testing.T) {
	e := setupWith(t, docFlagsApp())
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		doc, err := c.Insert(mustDoc(t, c, "Thing", Doc{"title": "G"}), SaveOpts{})
		if err != nil {
			return err
		}
		got, err := c.GetDoc("Thing", doc.ID())
		if err != nil {
			return err
		}
		if got.Str("seen") != "v" {
			t.Fatalf("onUpdate did not see validate's flag: seen=%q", got.Str("seen"))
		}
		// and nothing stands in for the caller's flag
		got["note"] = "locked"
		if _, err := c.Save(got, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("an unflagged write was not refused: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// #104: flags and __before come from the write, never from the document's
// data. A client's body is data: a "flags" key in it must not pass a rule
// keyed on doc.flags, nor a "__before" one stand in for the stored version.
func TestDocFlagsAreNotTakenFromTheData(t *testing.T) {
	e := setupWith(t, docFlagsApp())
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		forged := func(extra Doc) Doc {
			d := mustDoc(t, c, "Thing", Doc{"title": "F", "note": "locked"})
			for k, v := range extra {
				d[k] = v
			}
			return d
		}
		if _, err := c.Insert(forged(Doc{"flags": map[string]any{"system": true}}), SaveOpts{}); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("an insert whose data carried flags was not refused: %v", err)
		}
		d := forged(Doc{"note": "", "__before": map[string]any{"note": "forged"}})
		if _, err := c.Insert(d, SaveOpts{}); err != nil {
			t.Fatalf("an insert whose data carried __before saw it as the stored version: %v", err)
		}
		got, err := c.GetDoc("Thing", "F")
		if err != nil {
			return err
		}
		got["note"] = "locked"
		got["flags"] = map[string]any{"system": true}
		if _, err := c.Save(got, SaveOpts{}); err == nil || !strings.Contains(err.Error(), "refused") {
			t.Fatalf("a save whose data carried flags was not refused: %v", err)
		}
		if nd, err := c.NewDoc("Thing", Doc{"title": "N", "flags": map[string]any{"system": true}, "__before": map[string]any{}}); err != nil {
			return err
		} else if _, ok := nd["flags"]; ok {
			t.Fatalf("NewDoc kept a flags key: %v", nd)
		} else if _, ok := nd["__before"]; ok {
			t.Fatalf("NewDoc kept a __before key: %v", nd)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	out, _, err := e.Eval(context.Background(), `
const refused = (fn: () => void) => { try { fn(); return false; } catch (e) { return true; } };
const r: any = {};
r.newDoc = refused(() => ddcore.newDoc("Thing", { title: "J", note: "locked", flags: { system: true } } as any).insert());
r.wrapped = refused(() => ddcore.getDoc({ doctype: "Thing", title: "J", note: "locked", flags: { system: true } } as any).insert());
const before = ddcore.newDoc("Thing", { title: "K", __before: { note: "forged" } } as any);
r.before = refused(() => before.insert());
JSON.stringify(r);`, false)
	if err != nil {
		t.Fatal(err)
	}
	want := `"{\"newDoc\":true,\"wrapped\":true,\"before\":false}"`
	if string(out) != want {
		t.Fatalf("a document's data set its flags:\n got %s\nwant %s", out, want)
	}
}
