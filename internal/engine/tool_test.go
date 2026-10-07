package engine

import (
	"context"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// A tool Single (#114) is filled on screen and handed to a service: it reads
// as its defaults, and every write to it is refused, Admin's included.
func TestToolSingleRefusesEveryWrite(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/link_generator/link_generator.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Link Generator", label: "Link Generator", isSingle: true, tool: true,
  fields: [{ fieldname: "cpf", fieldtype: "Data", label: "CPF" }],
  permissions: [{ role: "Gestor", read: true }] });`,
	})
	err := e.Run(context.Background(), "Admin", func(c *Ctx) error {
		d, err := c.St.DocType("Link Generator")
		if err != nil {
			return err
		}
		if !d.Tool {
			t.Fatal("tool did not reach the meta")
		}
		doc, err := c.GetDoc("Link Generator", "singleton")
		if err != nil {
			t.Fatalf("a tool reads as its defaults: %v", err)
		}
		doc["cpf"] = "52998224725"
		if _, err := c.Save(doc, SaveOpts{}); cerr.From(err).Type != "ValidationError" {
			t.Errorf("Save: %v", err)
		}
		if _, err := c.Insert(doc, SaveOpts{IgnorePermissions: true}); cerr.From(err).Type != "ValidationError" {
			t.Errorf("Insert: %v", err)
		}
		if _, err := c.DBSet("Link Generator", "singleton", Doc{"cpf": "x"}, false); cerr.From(err).Type != "ValidationError" {
			t.Errorf("DBSet: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
