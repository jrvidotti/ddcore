package print

import (
	"strings"
	"testing"

	"github.com/jrvidotti/ddcore/internal/meta"
)

func TestRenderBlocks_AllTypesAndEscaping(t *testing.T) {
	blocks := []Block{
		{
			Type:       "header",
			Title:      "Sales <Invoice>",
			Subtitle:   "INV-001 & Special",
			Badge:      "Draft",
			BadgeColor: "orange",
		},
		{
			Type:    "keyValues",
			Columns: 2,
			Pairs: [][]string{
				{"Customer <Name>", "Alice & Bob"},
				{"Tax ID", "12.345.678/0001-90"},
			},
		},
		{
			Type:  "h",
			Level: 3,
			Text:  "Line <Items>",
		},
		{
			Type:    "table",
			Headers: []string{"Item", "Qty", "Price", "Amount"},
			Rows: [][]string{
				{"Widget <X>", "2", "R$ 150,00", "R$ 300,00"},
				{"Gadget <b>Bold</b>", "1", "R$ 450,00", "R$ 450,00"},
			},
			Aligns: []string{"left", "right", "right", "right"},
		},
		{
			Type: "totals",
			Pairs: [][]string{
				{"Subtotal", "R$ 750,00"},
				{"Grand Total", "R$ 750,00"},
			},
		},
		{
			Type: "p",
			Text: "Thank you for your business! <script>alert(1)</script>",
		},
		{
			Type: "rule",
		},
		{
			Type: "pageBreak",
		},
		{
			Type: "raw",
			HTML: `<div class="custom-badge">APPROVED</div>`,
		},
	}

	htmlOut := RenderBlocks(blocks)

	// Verify escaping of dangerous HTML in text blocks
	if strings.Contains(htmlOut, "<Invoice>") || strings.Contains(htmlOut, "<script>") || strings.Contains(htmlOut, "<b>Bold</b>") {
		t.Fatalf("HTML characters were not properly escaped in output: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "Sales &lt;Invoice&gt;") {
		t.Fatalf("expected escaped title, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "&lt;script&gt;alert(1)&lt;/script&gt;") {
		t.Fatalf("expected script tag to be escaped, got: %s", htmlOut)
	}

	// Verify structural components
	if !strings.Contains(htmlOut, `class="print-header"`) {
		t.Fatal("missing print-header class")
	}
	if !strings.Contains(htmlOut, `class="print-grid cols-2"`) {
		t.Fatal("missing print-grid class")
	}
	if !strings.Contains(htmlOut, `class="print-table"`) {
		t.Fatal("missing print-table class")
	}
	if !strings.Contains(htmlOut, `class="print-totals"`) {
		t.Fatal("missing print-totals class")
	}
	if !strings.Contains(htmlOut, `class="page-break"`) {
		t.Fatal("missing page-break class")
	}
	// Verify raw escape hatch was not escaped
	if !strings.Contains(htmlOut, `<div class="custom-badge">APPROVED</div>`) {
		t.Fatal("raw HTML escape hatch was modified")
	}
}

func TestStandardTemplate_AutoGenerationAndFieldOmission(t *testing.T) {
	doctype := &meta.DocType{
		Name:        "Purchase Order",
		Label:       "Purchase Order",
		Submittable: true,
		TitleField:  "supplier_name",
		Fields: []*meta.Field{
			{Fieldname: "supplier_name", Fieldtype: "Data", Label: "Supplier Name"},
			{Fieldname: "date", Fieldtype: "Date", Label: "Order Date"},
			{Fieldname: "secret_token", Fieldtype: "Password", Label: "Secret Token"},               // Must be omitted!
			{Fieldname: "vault_key", Fieldtype: "Vault", Label: "Vault Key"},                        // Must be omitted!
			{Fieldname: "internal_notes", Fieldtype: "Text", Label: "Internal Notes", Hidden: true}, // Hidden: omitted!
			{Fieldtype: "Section Break", Label: "Financial Details"},
			{Fieldname: "currency", Fieldtype: "Data", Label: "Currency"},
			{Fieldname: "total_amount", Fieldtype: "Currency", Label: "Total Amount"},
			{Fieldname: "items", Fieldtype: "Table", Label: "Order Items", Options: "Purchase Order Item"},
		},
	}

	childDoctype := &meta.DocType{
		Name:    "Purchase Order Item",
		IsChild: true,
		Fields: []*meta.Field{
			{Fieldname: "item_code", Fieldtype: "Data", Label: "Item Code", InListView: true},
			{Fieldname: "qty", Fieldtype: "Float", Label: "Quantity", InListView: true},
			{Fieldname: "rate", Fieldtype: "Currency", Label: "Rate", InListView: true},
		},
	}

	doc := map[string]any{
		"name":           "PO-2026-001",
		"docstatus":      1, // Submitted
		"supplier_name":  "Acme Corp",
		"date":           "2026-09-14",
		"secret_token":   "super-secret-password",
		"vault_key":      "vault:123",
		"internal_notes": "Do not show on print",
		"currency":       "BRL",
		"total_amount":   "1250.00",
		"items": []any{
			map[string]any{
				"item_code": "ITEM-A",
				"qty":       10,
				"rate":      "100.00",
			},
			map[string]any{
				"item_code": "ITEM-B",
				"qty":       5,
				"rate":      "50.00",
			},
		},
	}

	blocks := StandardTemplate(doctype, doc, StandardFormatOptions{
		GetChildMeta: func(dt string) *meta.DocType {
			if dt == "Purchase Order Item" {
				return childDoctype
			}
			return nil
		},
		FormatValue: func(f *meta.Field, val any) string {
			if f.Fieldname == "total_amount" {
				return "R$ 1.250,00"
			}
			return str(val)
		},
	})

	htmlOut := RenderBlocks(blocks)

	// Assert Document Header
	if !strings.Contains(htmlOut, "Purchase Order: Acme Corp") {
		t.Fatalf("expected title with supplier name, got: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "Submitted") {
		t.Fatalf("expected Submitted badge, got: %s", htmlOut)
	}

	// Assert sensitive fields are omitted
	if strings.Contains(htmlOut, "super-secret-password") || strings.Contains(htmlOut, "Secret Token") {
		t.Fatal("Password field was rendered in output!")
	}
	if strings.Contains(htmlOut, "vault:123") || strings.Contains(htmlOut, "Vault Key") {
		t.Fatal("Vault field was rendered in output!")
	}
	if strings.Contains(htmlOut, "Do not show on print") || strings.Contains(htmlOut, "Internal Notes") {
		t.Fatal("Hidden field was rendered in output!")
	}

	// Assert fields rendered
	if !strings.Contains(htmlOut, "R$ 1.250,00") {
		t.Fatalf("currency total_amount not formatted, got: %s", htmlOut)
	}

	// Assert child table rendered
	if !strings.Contains(htmlOut, "ITEM-A") || !strings.Contains(htmlOut, "ITEM-B") {
		t.Fatalf("child table items missing from output: %s", htmlOut)
	}
	if !strings.Contains(htmlOut, "Quantity") || !strings.Contains(htmlOut, "Rate") {
		t.Fatalf("child table headers missing from output: %s", htmlOut)
	}
}

func TestAssembleHTML_WithLetterheadAndStyles(t *testing.T) {
	lh := &LetterHead{
		Name:       "Acme Header",
		HeaderHTML: "<h3>ACME CORPORATION</h3><p>Tax ID: 00.000.000/0001-00</p>",
		FooterHTML: "<p>Page footer notes - Tel: +55 11 99999-9999</p>",
		Align:      "Center",
		Image:      "/files/logo.png",
	}

	body := `<div class="print-content">Hello World</div>`
	fullHTML := AssembleHTML(body, lh, "Test Document", "pt-BR", PDFOptions{})

	if !strings.Contains(fullHTML, `<html lang="pt-BR">`) {
		t.Fatalf("lang attribute missing or incorrect: %s", fullHTML)
	}
	if !strings.Contains(fullHTML, `<title>Test Document</title>`) {
		t.Fatalf("title missing or incorrect: %s", fullHTML)
	}
	if !strings.Contains(fullHTML, `@page {`) || !strings.Contains(fullHTML, `table.print-table thead {`) {
		t.Fatal("CSS print styles missing")
	}
	if !strings.Contains(fullHTML, `ACME CORPORATION`) || !strings.Contains(fullHTML, `align-center`) {
		t.Fatal("letterhead header missing or not centered")
	}
	if !strings.Contains(fullHTML, `/files/logo.png`) {
		t.Fatal("letterhead logo image missing")
	}
	if !strings.Contains(fullHTML, `Page footer notes`) {
		t.Fatal("letterhead footer missing")
	}
	if !strings.Contains(fullHTML, `Hello World`) {
		t.Fatal("document body missing")
	}
}

// A columns block carries its cells as lists of blocks, and each cell renders
// its blocks the same way the top level does, escaping included.
func TestRenderBlocks_Columns(t *testing.T) {
	blocks := []Block{{
		Type: "columns",
		Cells: [][]Block{
			{{Type: "h", Level: 3, Text: "Billed To"}, {Type: "p", Text: "Acme <Ltd>"}},
			{{Type: "keyValues", Columns: 2, Pairs: [][]string{{"Issue Date", "2026-09-14"}}}},
		},
	}}
	out := RenderBlocks(blocks)
	if !strings.Contains(out, `<div class="print-columns" style="grid-template-columns: repeat(2, 1fr)">`) {
		t.Fatalf("expected a two-column grid, got: %s", out)
	}
	if strings.Count(out, `<div class="print-column">`) != 2 {
		t.Fatalf("expected two cells, got: %s", out)
	}
	if !strings.Contains(out, "Billed To") || !strings.Contains(out, "Acme &lt;Ltd&gt;") || !strings.Contains(out, "Issue Date") {
		t.Fatalf("cell blocks missing or unescaped: %s", out)
	}
}

// The page size and orientation reach the HTML as an @page rule, which is what
// Chrome and a PDF command read; only Gotenberg also gets them as form fields.
func TestAssembleHTML_PageSize(t *testing.T) {
	cases := []struct {
		opts PDFOptions
		want string
	}{
		{PDFOptions{}, "size: A4 portrait;"},
		{PDFOptions{Format: "a4", Landscape: true}, "size: A4 landscape;"},
		{PDFOptions{Format: "Letter"}, "size: Letter portrait;"},
		{PDFOptions{Format: "LETTER", Landscape: true}, "size: Letter landscape;"},
	}
	for _, c := range cases {
		out := AssembleHTML("<p>x</p>", nil, "T", "en", c.opts)
		if !strings.Contains(out, c.want) {
			t.Fatalf("%+v: expected %q in: %s", c.opts, c.want, out)
		}
	}
}
