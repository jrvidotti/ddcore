package engine

import (
	"context"
	"strings"
	"testing"
)

func TestPrintDoc_StandardFormatAndSecurity(t *testing.T) {
	ctx := context.Background()
	e := setupWith(t, map[string]string{
		"doctypes/contract/contract.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Contract",
  label: "Contract",
  titleField: "client",
  fields: [
    { fieldname: "client", fieldtype: "Data", label: "Client", reqd: true },
    { fieldname: "amount", fieldtype: "Currency", label: "Amount" },
    { fieldname: "secret", fieldtype: "Password", label: "Secret Code" },
    { fieldname: "active", fieldtype: "Check", label: "Active" },
    { fieldname: "items", fieldtype: "Table", label: "Items", options: "Contract Item" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true },
    { role: "Sales User", read: true, write: true, create: true },
  ],
});`,
		"doctypes/contract_item/contract_item.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Contract Item",
  isChild: true,
  fields: [
    { fieldname: "description", fieldtype: "Data", label: "Description", inListView: true },
    { fieldname: "qty", fieldtype: "Int", label: "Qty", inListView: true },
  ],
});`,
		"doctypes/secret_doc/secret_doc.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Secret Doc",
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true },
  ],
});`,
	})

	// 1. Insert contract document
	var docName string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.Insert(Doc{
			"doctype": "Contract",
			"name":    "CTR-001",
			"client":  "Acme Brazil",
			"amount":  50000.50,
			"secret":  "topsecretpass",
			"active":  true,
			"items": []any{
				map[string]any{"description": "Consulting Services", "qty": 10},
				map[string]any{"description": "Maintenance", "qty": 5},
			},
		}, SaveOpts{})
		if err != nil {
			return err
		}
		docName = doc.Name()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// 2. Render standard format as Admin in pt-BR
	cAdmin := e.NewCtx(ctx, "Administrator")
	htmlOut, err := cAdmin.PrintDoc("Contract", docName, "standard", "none", "pt-BR")
	if err != nil {
		t.Fatal(err)
	}

	// Assertions on content
	if !strings.Contains(htmlOut, "Contract: Acme Brazil") {
		t.Fatalf("expected title with client name, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "Consulting Services") || !strings.Contains(htmlOut, "Maintenance") {
		t.Fatalf("missing child table rows in output: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "R$ 50.000,50") {
		t.Fatalf("currency not formatted properly in pt-BR: %s", htmlOut)
	}

	// Assert Password field is redacted
	if strings.Contains(htmlOut, "topsecretpass") || strings.Contains(htmlOut, "Secret Code") {
		t.Fatalf("Password field was rendered in output: %s", htmlOut)
	}

	// 3. Unauthorized access check
	// Insert Secret Doc
	err = e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Insert(Doc{
			"doctype": "Secret Doc",
			"name":    "SEC-001",
			"title":   "Top Secret Mission",
		}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Guest / unauthorized user trying to print Secret Doc
	cGuest := e.NewCtx(ctx, "Guest")
	_, err = cGuest.PrintDoc("Secret Doc", "SEC-001", "standard", "none", "en")
	if err == nil {
		t.Fatal("expected unauthorized user to fail printing Secret Doc, got nil")
	}
	if !strings.Contains(err.Error(), "permission") && !strings.Contains(err.Error(), "Permission") {
		t.Fatalf("expected permission error, got: %v", err)
	}
}

func TestPrintDoc_CustomAppTemplate(t *testing.T) {
	ctx := context.Background()
	e := setupWith(t, map[string]string{
		"doctypes/receipt/receipt.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Receipt",
  fields: [
    { fieldname: "donor", fieldtype: "Data", label: "Donor" },
    { fieldname: "amount", fieldtype: "Currency", label: "Amount" },
    { fieldname: "date", fieldtype: "Date", label: "Date" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }],
});`,
		"print/receipt_custom.print.ts": `import { definePrintTemplate, _ } from "@ddcore/sdk";
export default definePrintTemplate({
  name: "demo.receipt_custom",
  doctype: "Receipt",
  label: "Official Receipt",
  body: (doc, b, ctx) => [
    b.header(_("Official Receipt"), { subtitle: doc.name }),
    b.p(_("Received from {0}", [doc.donor])),
    b.keyValues([
      [_("Contribution"), ctx.formatCurrency(doc.amount)],
      [_("Date"), ctx.formatDate(doc.date)],
    ]),
  ],
});`,
	})

	var docName string
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		doc, err := c.Insert(Doc{
			"doctype": "Receipt",
			"name":    "REC-999",
			"donor":   "Carlos Silva",
			"amount":  250.00,
			"date":    "2026-09-14",
		}, SaveOpts{})
		if err != nil {
			return err
		}
		docName = doc.Name()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	cAdmin := e.NewCtx(ctx, "Administrator")
	// Check format listing
	formats, err := cAdmin.ListPrintFormats("Receipt")
	if err != nil {
		t.Fatal(err)
	}
	if len(formats) < 2 {
		t.Fatalf("expected at least 2 formats (standard + demo.receipt_custom), got: %+v", formats)
	}

	// Render custom template
	htmlOut, err := cAdmin.PrintDoc("Receipt", docName, "demo.receipt_custom", "none", "pt-BR")
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(htmlOut, "Official Receipt") {
		t.Fatalf("expected Official Receipt header, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "Carlos Silva") {
		t.Fatalf("expected donor name, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "250,00") {
		t.Fatalf("expected formatted amount, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "14/09/2026") {
		t.Fatalf("expected formatted date in pt-BR, got: %s", htmlOut)
	}
}
