package engine

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jrvidotti/ddcore/internal/meta"
	"github.com/jrvidotti/ddcore/internal/num"
	"github.com/jrvidotti/ddcore/internal/print"
)

// The server prints money with the places it is given — a field kept in
// whole units, or a site whose currencyPrecision overrides the currency's —
// not the currency's ISO minor unit, which it used to fall back to (#159).
func TestFormatCurrencyHonorsPlaces(t *testing.T) {
	half := num.HalfAwayFromZero
	for _, c := range []struct {
		why, code, lang string
		v               float64
		places          int
		want            string
	}{
		{"the site's two places, as before", "BRL", "pt-BR", 6000.376, 2, "R$ 6.000,38"},
		{"whole units", "BRL", "pt-BR", 6000.376, 0, "R$ 6.000"},
		{"more places than the ISO unit", "USD", "en", 1.234567, 4, "$ 1.2346"},
		{"the language groups, the currency names", "USD", "pt-BR", 50000.5, 2, "US$ 50.000,50"},
		{"a yen with places a site asked for", "JPY", "en", 1234.5, 2, "¥ 1,234.50"},
		{"negative", "BRL", "pt-BR", -6000.376, 2, "R$ -6.000,38"},
		{"no minus on a value that rounded to zero", "BRL", "pt-BR", -0.001, 2, "R$ 0,00"},
		{"an unknown code prints as itself", "XYZ", "en", 1234.5, 2, "XYZ 1,234.50"},
	} {
		if got := FormatCurrency(c.v, c.code, c.lang, c.places, half); got != c.want {
			t.Errorf("%s: got %q, want %q", c.why, got, c.want)
		}
	}
	// the site's rounding rule, not the formatter's own
	if got := FormatCurrency(2.5, "BRL", "pt-BR", 0, num.HalfToEven); got != "R$ 2" {
		t.Errorf("bankers: got %q", got)
	}
	if got := FormatCurrency(2.5, "BRL", "pt-BR", 0, half); got != "R$ 3" {
		t.Errorf("commercial: got %q", got)
	}
}

const budgetDoctype = `import { defineDoctype } from "@ddcore/sdk";
export default defineDoctype({
  name: "Budget",
  fields: [
    { fieldname: "amount", fieldtype: "Currency", precision: 0, label: "Amount" },
    { fieldname: "fee", fieldtype: "Currency", label: "Fee" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true }],
});`

// The standard layout, a print template's ctx.formatCurrency and
// ddcore.utils.formatCurrency all print a whole-units field without cents.
func TestPrintAndUtilsHonorAFieldsPrecision(t *testing.T) {
	ctx := context.Background()
	e := setupWith(t, map[string]string{
		"doctypes/budget/budget.doctype.ts": budgetDoctype,
		"print/budget_custom.print.ts": `import { definePrintTemplate } from "@ddcore/sdk";
export default definePrintTemplate({
  name: "demo.budget_custom",
  doctype: "Budget",
  label: "Budget",
  body: (doc, b, ctx) => [b.p("amount=" + ctx.formatCurrency(doc.amount, 0) + ";fee=" + ctx.formatCurrency(doc.fee))],
});`,
		"services/money.ts": `export function fmt() {
  return [ddcore.utils.formatCurrency(6000.376), ddcore.utils.formatCurrency(6000.376, undefined, 0), ddcore.utils.formatCurrency(1234.5, "JPY")].join("|");
}`,
	})
	e.Cfg.Currency = "BRL"
	err := e.Run(ctx, "Admin", func(c *Ctx) error {
		_, err := c.Insert(Doc{"doctype": "Budget", "id": "B-1", "amount": 6000.376, "fee": 12.345}, SaveOpts{})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	out, err := e.NewCtx(ctx, "Admin").PrintDoc("Budget", "B-1", "standard", "none", "pt-BR", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "R$ 6.000<") || strings.Contains(out, "6.000,") {
		t.Errorf("standard layout: whole units expected, got: %s", out)
	}
	if !strings.Contains(out, "R$ 12,35") {
		t.Errorf("standard layout: a field without precision keeps the site's, got: %s", out)
	}

	out, err = e.NewCtx(ctx, "Admin").PrintDoc("Budget", "B-1", "demo.budget_custom", "none", "pt-BR", print.PDFOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "amount=6.000;fee=12,35") {
		t.Errorf("ctx.formatCurrency: got: %s", out)
	}

	res, err := e.RunJob(ctx, "Admin", "demo.services.money.fmt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := `"R$ 6.000,38|R$ 6.000|JP¥ 1.235"`; !strings.Contains(string(res), want) {
		t.Errorf("ddcore.utils.formatCurrency: got %s, want %s", res, want)
	}
}

// An XLSX shows a Currency column with its field's places, in the desk's
// export and in ddcore.spreadsheet.file alike.
func TestXLSXHonorsACurrencyColumnsPrecision(t *testing.T) {
	d := &meta.DocType{Name: "Budget", Fields: []*meta.Field{
		{Fieldname: "amount", Fieldtype: "Currency", Label: "Amount", Precision: new(0)},
		{Fieldname: "fee", Fieldtype: "Currency", Label: "Fee"},
	}}
	var buf bytes.Buffer
	s := &XLSXSink{Open: func(string) (io.Writer, error) { return &buf, nil }, MoneyDecimals: 2}
	tw, err := s.openXLSX("Budget", d, []string{"amount", "fee"})
	if err != nil {
		t.Fatal(err)
	}
	if err := tw.Write([]any{6000.0, 12.35}); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	sheet := zipText(t, buf.Bytes(), "xl/worksheets/sheet1.xml")
	for _, want := range []string{`<c r="A2" s="6"><v>6000</v></c>`, `<c r="B2" s="5"><v>12.35</v></c>`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("export lacks %s:\n%s", want, sheet)
		}
	}

	buf.Reset()
	sheets := []SpreadsheetSheet{{Name: "Grade", Columns: []SpreadsheetColumn{
		{Label: "Amount", Fieldtype: "Currency", Precision: new(0)},
		{Label: "Fee", Fieldtype: "Currency"},
	}, Rows: [][]any{{6000.0, 12.35}}}}
	if err := writeSpreadsheetXLSX(&buf, sheets, time.UTC, 2); err != nil {
		t.Fatal(err)
	}
	sheet = zipText(t, buf.Bytes(), "xl/worksheets/sheet1.xml")
	for _, want := range []string{`<c r="A2" s="6"><v>6000</v></c>`, `<c r="B2" s="5"><v>12.35</v></c>`} {
		if !strings.Contains(sheet, want) {
			t.Errorf("spreadsheet lacks %s:\n%s", want, sheet)
		}
	}
}
