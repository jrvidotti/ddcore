package engine

import (
	"context"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// A Transfer is made by a server method, never typed in: allowCreate: false
// takes create and amend away from the desk, Admin included, and a
// spreadsheet insert is refused, while server code still inserts.
func TestAllowCreateFalse(t *testing.T) {
	e := setupWith(t, map[string]string{
		"doctypes/transfer/transfer.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({ name: "Transfer", submittable: true, allowCreate: false,
  description: "Made with the Transfer button on the source account's form",
  fields: [{ fieldname: "amount", fieldtype: "Int", label: "Amount" }],
  permissions: [{ role: "Gestor", read: true, write: true, create: true, submit: true, cancel: true, amend: true, import: true }] });`,
	})
	ctx := context.Background()
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		d, err := c.St.DocType("Transfer")
		if err != nil {
			return err
		}
		p := c.Permissions(d)
		if p["create"] || p["amend"] {
			t.Errorf("Admin: create=%v amend=%v, want both false", p["create"], p["amend"])
		}
		if !p["read"] || !p["write"] || !p["submit"] {
			t.Errorf("Admin keeps the other permissions: %v", p)
		}
		u, err := c.St.DocType("User")
		if err != nil {
			return err
		}
		if !c.Permissions(u)["create"] {
			t.Error("a DocType without allowCreate stays creatable")
		}

		doc, _ := c.NewDoc("Transfer", Doc{"amount": 10})
		if _, err := c.Insert(doc, SaveOpts{}); err != nil {
			t.Errorf("server code inserts: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	c := e.NewCtx(ctx, "Admin")
	c.Lang = "en"
	_, err = c.DataImport(DataImportArgs{Doctype: "Transfer", FileName: "rows.csv", File: []byte("Amount\n5\n"), DryRun: true})
	if cerr.From(err).Type != "ValidationError" {
		t.Fatalf("insert from a spreadsheet: %v", err)
	}
	if _, err = c.DataImport(DataImportArgs{Doctype: "Transfer", Mode: "update", FileName: "rows.csv", File: []byte("id,Amount\nx,5\n"), DryRun: true}); err != nil {
		t.Fatalf("update from a spreadsheet: %v", err)
	}
}
