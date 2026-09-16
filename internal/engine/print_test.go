package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/cerr"
	"github.com/jrvidotti/ddcore/internal/print"
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
	htmlOut, err := cAdmin.PrintDoc("Contract", docName, "standard", "none", "pt-BR", print.PDFOptions{})
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
	_, err = cGuest.PrintDoc("Secret Doc", "SEC-001", "standard", "none", "en", print.PDFOptions{})
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
	htmlOut, err := cAdmin.PrintDoc("Receipt", docName, "demo.receipt_custom", "none", "pt-BR", print.PDFOptions{})
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

// The columns example of docs/agent/print.md, run through the engine: the
// builder's output must unmarshal into print.Block and render every cell.
func TestPrintDoc_ColumnsTemplate(t *testing.T) {
	ctx := context.Background()
	e := setupWith(t, map[string]string{
		"doctypes/invoice/invoice.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Invoice",
  fields: [
    { fieldname: "customer_name", fieldtype: "Data", label: "Customer Name" },
    { fieldname: "posting_date", fieldtype: "Date", label: "Posting Date" },
    { fieldname: "grand_total", fieldtype: "Currency", label: "Grand Total" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }],
});`,
		"print/invoice_columns.print.ts": `import { definePrintTemplate, _ } from "@ddcore/sdk";
export default definePrintTemplate({
  name: "demo.invoice_columns",
  doctype: "Invoice",
  label: "Invoice with columns",
  body: (doc, b, ctx) => [
    b.header(doc.name, { subtitle: _("Invoice") }),
    b.columns([
      [b.h(3, _("Billed To")), b.p(doc.customer_name)],
      [b.keyValues([[_("Posting Date"), ctx.formatDate(doc.posting_date)]])],
    ]),
    b.rule(),
    b.totals([[_("Grand Total"), ctx.formatCurrency(doc.grand_total)]]),
  ],
});`,
	})
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Insert(Doc{"doctype": "Invoice", "name": "INV-1", "customer_name": "Acme <Ltd>", "posting_date": "2026-09-14", "grand_total": 10}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := e.NewCtx(ctx, "Administrator").PrintDoc("Invoice", "INV-1", "demo.invoice_columns", "none", "en", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`class="print-columns"`, "Billed To", "Acme &lt;Ltd&gt;", "Posting Date", "2026-09-14", "Grand Total"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in: %s", want, out)
		}
	}
}

// The standard layout prints money in the site currency, with the grouping
// and decimal mark of the print language: the language does not pick the
// currency.
func TestPrintDoc_CurrencyFollowsSiteCurrency(t *testing.T) {
	ctx := context.Background()
	e := setupWith(t, map[string]string{
		"doctypes/fee/fee.doctype.ts": `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Fee",
  fields: [{ fieldname: "amount", fieldtype: "Currency", label: "Amount" }],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }],
});`,
	})
	e.Cfg.Currency = "USD"
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		_, err := c.Insert(Doc{"doctype": "Fee", "name": "FEE-1", "amount": 50000.5}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for lang, want := range map[string]string{"en": "$ 50,000.50", "pt-BR": "US$ 50.000,50"} {
		out, err := e.NewCtx(ctx, "Administrator").PrintDoc("Fee", "FEE-1", "standard", "none", lang, print.PDFOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, want) || strings.Contains(out, "R$") {
			t.Fatalf("%s: expected %q, got: %s", lang, want, out)
		}
	}
}

// Saving a Letter Head as the default clears the flag on every other one, so
// "the default" names one record.
func TestLetterHead_OneDefault(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		for _, n := range []string{"First", "Second"} {
			if _, err := c.Insert(Doc{"doctype": "Letter Head", "letter_head_name": n, "is_default": true}, SaveOpts{}); err != nil {
				return err
			}
		}
		first, err := c.GetDoc("Letter Head", "First")
		if err != nil {
			return err
		}
		if first["is_default"] == true {
			t.Fatalf("saving Second as default left First as default too")
		}
		first["is_default"] = true
		if _, err := c.Save(first, SaveOpts{}); err != nil {
			return err
		}
		second, err := c.GetDoc("Letter Head", "Second")
		if err != nil {
			return err
		}
		if second["is_default"] == true {
			t.Fatalf("saving First as default left Second as default too")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// With no letterhead named, the default is used, and the choice among several
// defaults written behind the controller's back is the most recently modified
// one, not whatever order the table returns. A named letterhead that does not
// exist or is disabled is an error, not a print without one.
func TestPrintDoc_LetterHeadSelection(t *testing.T) {
	ctx := context.Background()
	e := setup(t)
	err := e.Run(ctx, "Administrator", func(c *Ctx) error {
		for _, lh := range []Doc{
			{"doctype": "Letter Head", "letter_head_name": "Old", "header_html": "<b>OLD-HEADER</b>"},
			{"doctype": "Letter Head", "letter_head_name": "New", "header_html": "<b>NEW-HEADER</b>"},
			{"doctype": "Letter Head", "letter_head_name": "Off", "header_html": "<b>OFF-HEADER</b>", "disabled": true},
		} {
			if _, err := c.Insert(lh, SaveOpts{}); err != nil {
				return err
			}
		}
		// Old is updated first, so a scan returns it first; New is the newer one.
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_letter_head SET is_default = true, modified = now() - interval '1 hour' WHERE name = 'Old'`); err != nil {
			return err
		}
		if _, err := c.Q().Exec(c.Ctx, `UPDATE tab_letter_head SET is_default = true, modified = now() WHERE name = 'New'`); err != nil {
			return err
		}
		_, err := c.Insert(Doc{"doctype": "Pessoa", "nome": "Lia", "tipo": "PF"}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	c := e.NewCtx(ctx, "Administrator")
	out, err := c.PrintDoc("Pessoa", "Lia", "standard", "", "en", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "NEW-HEADER") || strings.Contains(out, "OLD-HEADER") {
		t.Fatalf("expected the most recently modified default letterhead: %s", out)
	}
	for _, name := range []string{"Missing", "Off"} {
		_, err := c.PrintDoc("Pessoa", "Lia", "standard", name, "en", print.PDFOptions{})
		if err == nil || cerr.From(err).Type != "ValidationError" {
			t.Fatalf("letterhead %q: expected ValidationError, got %v", name, err)
		}
	}
}
