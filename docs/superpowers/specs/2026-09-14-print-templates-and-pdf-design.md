# Design: Print Templates and PDF (OPS-01)

Record of the design built on 2026-09-14. The developer documentation will live in
[`docs/agent/print.md`](../../agent/print.md); this document preserves **why** each
component is architected the way it is.

---

## 1. Starting Point & Requirements

Stage 1 of the ddcore feature roadmap introduces broader administrative application
support. Receipts, contracts, invoices, order confirmations, and tax reports require
high-fidelity printed output and PDF generation:

1. **Default / Standard Print Format**: Automatically generated from DocType metadata
   (`meta.DocType`) for any document, laying out sections, columns, formatted field values,
   and child tables without requiring app developers to write custom templates.
2. **App-Declared Templates**: Defined in `print/<name>.print.ts` using `definePrintTemplate`
   in `@ddcore/sdk`. Runs synchronously on the server in goja, taking advantage of
   literal `_("…")` translations, escaping, and formatting helpers.
3. **Desk Print Experience**: Dedicated preview route (`/app/.../[doctype]/[name]/print`)
   featuring a paper preview canvas, format selector, letterhead picker, language switcher,
   and instant browser printing (`window.print()`).
4. **Pluggable PDF Generation**: Hybrid engine supporting Headless Chromium (CLI / CDP),
   Gotenberg / external HTTP services, or custom command adapters, with graceful degradation.
5. **Branding & Letterheads**: Standard Core DocType `Letter Head` (`tab_letter_head`)
   holding corporate logos, header HTML, and footer HTML.
6. **Strict Security**: Enforces document-level read permissions, User Access Scopes (SEC-01),
   redacts `Password` and `Vault` fields, and omits restricted fields (`permlevel > 0`).
7. **Acceptance Focus**: Long tables spanning pages with repeating headers, accented
   characters, formatted currency/numbers, timezone dates, page breaks, and unauthorized denial.

---

## 2. Key Architecture Decisions

### Decision 1: Hybrid Print & PDF Pipeline
- **Desk Preview & Native Printing**: The server renders self-contained HTML with print
  CSS (`@page`, `@media print`). Desk renders this in a sandboxed iframe. Clicking "Print"
  triggers `iframe.contentWindow.print()`, leveraging the user's browser for instant,
  100% accurate printing or saving as PDF with zero server CPU/RAM overhead.
- **Server-Side PDF Generation**: For programmatic workflows (e.g. attaching an invoice
  to an email or downloading via `/api/print/{doctype}/{name}/pdf`), the server uses a
  pluggable `PDFRenderer` interface with adapters for:
  - **Headless Chromium**: Discovers `google-chrome` or `chromium` on the host, running
    `--headless=new --print-to-pdf-no-header`.
  - **Gotenberg**: Multi-container deployments post HTML to Gotenberg's conversion endpoint.
  - **Command**: Configurable CLI command (`DDCORE_PDF_COMMAND`) for custom tools.
  - **Graceful Fallback**: If no PDF renderer is installed, the API returns a descriptive
    validation error guiding users to use browser print or configure Chromium.

### Decision 2: Structured Print Block Vocabulary + Standard Format
Like mail templates (`OPS-02`), print templates return a structured list of blocks
rather than unescaped raw HTML:
- Blocks: `b.header`, `b.keyValues`, `b.section`, `b.table`, `b.totals`, `b.p`, `b.h`,
  `b.rule`, `b.pageBreak()`, and `b.raw(html)` for custom elements (e.g., barcodes).
- Escaping is performed strictly by the core renderer in Go/TS, preventing XSS vulnerabilities.
- Standard format is generated automatically by inspecting the DocType's sections,
  columns, and child table fields.

### Decision 3: Standard DocType `Letter Head`
A standard Core DocType `Letter Head` (`tab_letter_head`) provides corporate branding:
- Fields: `name`, `is_default`, `disabled`, `header_html`, `footer_html`, `image`, `align`.
- Authenticated users can read letterheads to preview documents; `System Manager` manages them.
- Rendered in headers/footers with appropriate page margins.

### Decision 4: Authorization and Field Omission
The print renderer acts as a secured boundary:
- Checks `c.HasPermission(doctype, "read", doc)`. If false, returns `403 PermissionError`.
- Validates User Access Scopes (SEC-01) using `c.checkUserPermissions(d, doc)`.
- Strips `Password` and `Vault` fields unconditionally.
- Evaluates field permissions (`permlevel > 0`): fields the current user cannot read
  are stripped before the template receives the document.
- Child tables inherit row-level scope checks and field permissions.

### Decision 5: Internationalization, Accents, and Formatting
- Context helper `ctx` passed to template functions provides:
  - `ctx.formatCurrency(val)` (respecting site currency and precision from DAT-06).
  - `ctx.formatDate(val)` and `ctx.formatDateTime(val)` (in site/user timezone).
  - `ctx.formatNumber(val, decimals)`.
- Dynamic language resolution: evaluated inside goja with `globalThis.__ddcoreLang` set
  to the recipient's or selected language, ensuring all `_("…")` calls translate properly.
- Rune-based calculations and full UTF-8 Unicode support prevent text corruption or
  misalignment for accented strings.

---

## 3. Data Model & Metadata

### `Letter Head` DocType (`tab_letter_head`)
```typescript
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Letter Head",
  module: "Core",
  label: "Letter Head",
  icon: "file-text",
  naming: { field: "name" },
  searchFields: ["name"],
  fields: [
    { fieldname: "name", fieldtype: "Data", label: "Letter Head Name", reqd: true, inListView: true },
    { fieldname: "is_default", fieldtype: "Check", label: "Is Default", default: false, inListView: true },
    { fieldname: "disabled", fieldtype: "Check", label: "Disabled", default: false, inListView: true },
    { fieldname: "align", fieldtype: "Select", label: "Align", options: "Left\nCenter\nRight", default: "Left" },
    { fieldname: "image", fieldtype: "Attach", label: "Logo Image" },
    { fieldname: "header_html", fieldtype: "Code", label: "Header HTML", options: "HTML" },
    { fieldname: "footer_html", fieldtype: "Code", label: "Footer HTML", options: "HTML" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true },
    { role: "All", read: true },
  ],
});
```

---

## 4. HTTP API Endpoints

1. `GET /api/print/formats/{doctype}`:
   - Returns: `[ { name: "standard", label: "Standard", default: true }, ... ]`
2. `GET /api/print/{doctype}/{name}`:
   - Query: `format`, `lang`, `letterhead`, `raw=1`
   - Returns: Rendered HTML document with print stylesheet.
3. `GET /api/print/{doctype}/{name}/pdf`:
   - Query: `format`, `lang`, `letterhead`, `download=1`
   - Returns: Binary PDF with `Content-Type: application/pdf`.
4. `GET /api/letterheads`:
   - Returns active letterheads for Desk dropdowns.

---

## 5. Desk User Interface

- Route: `desk/src/routes/app/[workspace]/[doctype]/[name]/print/+page.svelte` (and non-workspace variant).
- Toolbar:
  - Back navigation to document form.
  - Print Format selector dropdown.
  - Letterhead selector dropdown.
  - Language selector dropdown.
  - Primary "Print" button (`window.print()`).
  - Secondary "Download PDF" button.
- Preview Canvas:
  - Sandboxed iframe showing the rendered A4 document with shadow and realistic margins.

---

## 6. Verification & Test Plan

1. **Engine & Template Tests (`internal/print/`)**:
   - Auto-generated standard layout for various DocTypes and child tables.
   - App template execution in goja with context helpers.
   - Accented strings, currency formatting, and timezone handling.
   - Long tables with repeating headers and avoid-break rows.
2. **Security Tests (`internal/api/print_test.go`)**:
   - Unauthorized user rejection (403).
   - User Access Scope isolation (SEC-01).
   - Redaction of passwords, vault secrets, and restricted fields.
3. **PDF Renderer Tests**:
   - Subprocess execution, timeouts, temporary file cleanup, and concurrency limit.
4. **Desk Tests**:
   - SvelteKit print view component tests (`npm run test`).
5. **System Validation**:
   - `make test` and `make check`.
