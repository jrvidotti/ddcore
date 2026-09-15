# Print templates and PDF

ddcore renders documents into printable HTML and PDFs — either via an auto-generated
standard layout derived from DocType metadata, or via custom app-declared print templates
written in TypeScript.

---

## 1. The standard print format

Every DocType automatically has a `"standard"` print format. No template code is needed.

The standard layout inspects the DocType metadata at render time:
- **Header:** Document title and primary status/indicator.
- **Sections and columns:** Mirrors the DocType's section breaks and column layout.
- **Fields:** Renders labels and formatted values (Currency, Date, Percent, Check, etc.) in the user's selected language.
- **Child tables:** Automatically rendered as responsive data tables with columns marked `inListView: true`.
- **Letter Head:** Embedded header and footer HTML if configured.

---

## 2. Declaring custom print templates

An app defines custom print templates in `print/<name>.print.ts`:

```ts
import { definePrintTemplate, _ } from "@ddcore/sdk";

export default definePrintTemplate({
  name: "sales.invoice_official",
  doctype: "Invoice",
  label: "Official Tax Invoice",
  body: (doc, b, ctx) => [
    b.header(doc.title || doc.name, {
      subtitle: _("Invoice #{0}", [doc.name]),
    }),
    b.columns([
      [
        b.h3(_("Billed To")),
        b.p(doc.customer_name),
        b.p(doc.customer_address),
      ],
      [
        b.h3(_("Invoice Details")),
        b.keyValues([
          [_("Issue Date"), ctx.formatDate(doc.posting_date)],
          [_("Due Date"), ctx.formatDate(doc.due_date)],
          [_("Currency"), doc.currency],
        ]),
      ],
    ]),
    b.divider(),
    b.table(
      [_("Item"), _("Qty"), _("Rate"), _("Amount")],
      (doc.items || []).map((item: any) => [
        item.description,
        item.qty,
        ctx.formatCurrency(item.rate, doc.currency),
        ctx.formatCurrency(item.amount, doc.currency),
      ])
    ),
    b.keyValues([
      [_("Subtotal"), ctx.formatCurrency(doc.subtotal, doc.currency)],
      [_("Taxes"), ctx.formatCurrency(doc.taxes, doc.currency)],
      [_("Grand Total"), ctx.formatCurrency(doc.grand_total, doc.currency)],
    ]),
  ],
});
```

### The Block Builder (`b`)

The print block builder provides typed, auto-escaping components designed for CSS paged media:

- `b.header(title, opts?)`: Document header with optional subtitle and logo URL.
- `b.h1(text)`, `b.h2(text)`, `b.h3(text)`: Section headings.
- `b.p(text)`: Paragraph of text.
- `b.divider()`: Horizontal rule separator.
- `b.keyValues([[key, value], ...])`: Two-column labeled metadata block.
- `b.table(headers, rows)`: Formatted data table.
- `b.columns(columnBlocks)`: Multi-column side-by-side layout.
- `b.html(safeHtml)`: Raw unescaped HTML when specific custom markup is required.

### The Print Context (`ctx`)

`ctx` provides locale-aware formatting utilities:
- `ctx.formatCurrency(value, currency?)`: Formats numeric amounts using site or document currency and user locale (e.g. `R$ 1.500,00` or `$1,500.00`).
- `ctx.formatDate(value)`: Formats civil dates (e.g. `14/09/2026`).
- `ctx.formatDatetime(value)`: Formats timestamps in the site timezone.
- `ctx.lang`: The target language code (`"pt-BR"`, `"en"`, etc.).

---

## 3. Letter Heads (Corporate Branding)

Corporate branding is managed through the standard Core DocType `Letter Head` (`tab_letter_head`).
A letterhead defines:
- `letter_head_name`: Unique name (e.g. "Corporate Standard").
- `header_html`: HTML/CSS rendered at the top of every printed page.
- `footer_html`: HTML/CSS rendered at the bottom of every printed page.
- `is_default`: Set to true for the default letterhead.
- `disabled`: Whether this letterhead is disabled.

When rendering:
- A specific letterhead can be passed via `letterhead=<name>`.
- If omitted, the default active letterhead is used.
- Pass `letterhead=none` to suppress letterhead headers and footers (e.g., printing onto pre-printed stationary).

---

## 4. Server-side PDF Generation

ddcore includes a pluggable PDF generation subsystem:

1. **Gotenberg (Recommended for Production & Docker):**
   Set `DDCORE_GOTENBERG_URL` (e.g. `http://gotenberg:3000`). ddcore posts HTML to Gotenberg's Chromium conversion endpoint.
2. **Custom Command:**
   Set `DDCORE_PDF_COMMAND` (e.g. `weasyprint {in} {out}` or `wkhtmltopdf {in} {out}`). Placeholders `{in}` and `{out}` are replaced with temp file paths.
3. **Local Headless Chrome / Chromium:**
   If neither environment variable is set, ddcore automatically discovers installed Chrome or Chromium binaries on the host system (`/Applications/Google Chrome.app`, `/usr/bin/google-chrome`, `chromium`, etc.) and executes them with `--headless --print-to-pdf`.
4. **Graceful Fallback:**
   If no PDF generator is available, `/api/print/.../pdf` returns HTTP 503 (`UnavailableError`). The Desk print preview UI gracefully prompts the user to use the browser's native Print to PDF dialog.

### Concurrency Protection

PDF generation is resource-intensive. ddcore protects system memory with a built-in concurrency semaphore (default max 3-5 concurrent renders). Excessive concurrent requests wait on a semaphore slot before running.

---

## 5. Security and Permissions

Printing strictly enforces document security:

1. **Document Read Permission:** Generating a print format or PDF checks `read` permission on the document for the current session/user. Unauthorized requests return HTTP 403 `PermissionError`.
2. **User Access Scopes (SEC-01):** Role constraints and ownership restrictions (`ifOwner`) are enforced.
3. **Field Redaction:** Sensitive fields are stripped before template execution:
   - All `Password` fields are omitted.
   - All `Vault` secrets are redacted.
   - Restricted or hidden fields are never leaked.

---

## 6. HTTP API

| Endpoint | Method | Description |
|---|---|---|
| `/api/print/formats/{doctype}` | GET | Returns available formats: `[{name, label, default}]` |
| `/api/letterheads` | GET | Returns active letterheads: `[{name, is_default}]` |
| `/api/print/{doctype}/{name}` | GET | Renders print HTML (`format`, `letterhead`, `lang`) |
| `/api/print/{doctype}/{name}/pdf` | GET | Streams rendered PDF (`download=1` for attachment) |

---

## 7. Desk Print Preview

Users can preview documents before printing:
- Accessible from the Form view via the **Print** button or the `...` actions menu.
- URL route: `/app/[workspace]/[doctype]/[name]/print`.
- Interactive toolbar allows live switching of:
  - Print Format (Standard vs custom templates)
  - Letter Head (Default, specific, or None)
  - Language
- Instant action buttons:
  - **Print:** Triggers `window.print()` targeting the isolated paper canvas.
  - **Download PDF:** Downloads server-rendered PDF with graceful browser-print fallback.
