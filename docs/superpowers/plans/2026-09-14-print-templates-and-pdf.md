# Print Templates and PDF (OPS-01) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement comprehensive document printing and PDF generation for ddcore, including auto-generated standard formats, app-declared templates (`definePrintTemplate`), Letter Head branding, strict security/field omission, Desk print preview, and pluggable server PDF generation.

**Architecture:** Print formats are defined via `definePrintTemplate` in `print/<name>.print.ts` or auto-generated from DocType metadata. The server renders structured, escaped blocks to clean HTML with print CSS (`@page`, `@media print`). Desk embeds this in an iframe for paper preview and zero-overhead `window.print()`. Server-side PDF generation is handled by a pluggable `PDFRenderer` supporting Headless Chromium, Gotenberg, or CLI tools with concurrency bounding and graceful fallback. Strict authorization enforces read permissions, User Access Scopes (SEC-01), and redacts confidential/restricted fields.

**Tech Stack:** Go 1.26, PostgreSQL, TypeScript, goja runtime, SvelteKit (Desk), Headless Chromium / Gotenberg.

**Spec:** [`docs/superpowers/specs/2026-09-14-print-templates-and-pdf-design.md`](../specs/2026-09-14-print-templates-and-pdf-design.md)

## Global Constraints
- Strictly adhere to [`AGENTS.md`](../../AGENTS.md): synchronous server TypeScript (goja), English canonical strings with translations, generated typings, zero external production dependencies for the core binary.
- Strict authorization: `HasPermission(doctype, "read", doc)` and `checkUserPermissions` must be checked before rendering. Unauthorized users receive HTTP 403.
- Restricted fields (`permlevel > 0` where unauthorized), `Password`, and `Vault` fields must be omitted/redacted from the rendered document.
- Long tables must support repeating headers (`thead { display: table-header-group }`) and prevent broken rows (`page-break-inside: avoid`).
- Unicode accents and currency must format cleanly without byte-level truncation or layout misalignment.

---

## Proposed Changes

### Component 1: Core Metadata (`Letter Head` DocType)
#### [NEW] `core/doctypes/letter_head/letter_head.doctype.ts`
Standard DocType storing corporate branding (name, default status, header HTML, footer HTML, logo, alignment).
#### [MODIFY] `core/translations/pt-BR.csv`
Translations for Letter Head fields and labels.

---

### Component 2: SDK Typings & Prelude Registration
#### [MODIFY] `packages/sdk/src/index.ts`
Exports `definePrintTemplate`.
#### [MODIFY] `packages/sdk/src/types.ts`
Type definitions for `PrintTemplateDef`, `PrintBlock`, `PrintContext`.
#### [MODIFY] `internal/js/prelude.js`
Registers `print` templates in `reg.printTemplates` and exposes `Snapshot.PrintTemplates` to Go.

---

### Component 3: Print Engine & Block Renderer
#### [NEW] `internal/print/blocks.go`
Print block definitions (`header`, `keyValues`, `section`, `table`, `totals`, `p`, `h`, `rule`, `pageBreak`, `raw`) with strict HTML escaping and layout rendering.
#### [NEW] `internal/print/standard.go`
Auto-generates standard document print layout by inspecting `meta.DocType` sections, fields, and child tables.
#### [NEW] `internal/print/render.go`
Assembles complete HTML document including print stylesheet, letterhead, and document body.
#### [NEW] `internal/print/render_test.go`
Unit tests for blocks, tables, runes/accents, auto-format, escaping, and formatting.

---

### Component 4: Engine Integration & Security (SEC-01/02)
#### [MODIFY] `internal/engine/engine.go`
Includes `PrintTemplates` in `Snapshot`.
#### [NEW] `internal/engine/print.go`
Engine `PrintDoc` method: checks document read permissions, user access scopes, filters restricted fields, resolves letterheads, and evaluates templates in goja.
#### [NEW] `internal/engine/print_test.go`
Tests for permission denials, user scope isolation, and restricted field redaction.

---

### Component 5: PDF Rendering Subsystem
#### [NEW] `internal/print/pdf.go`
`PDFRenderer` interface with Headless Chromium, Gotenberg, and CLI adapters, plus concurrency limiting semaphore.
#### [NEW] `internal/print/pdf_test.go`
Tests for discovery, execution, timeouts, cleanup, and graceful error handling.

---

### Component 6: HTTP API Endpoints
#### [NEW] `internal/api/print.go`
API endpoints:
- `GET /api/print/formats/{doctype}`
- `GET /api/print/{doctype}/{name}`
- `GET /api/print/{doctype}/{name}/pdf`
- `GET /api/letterheads`
#### [MODIFY] `internal/api/api.go`
Mounts print and letterhead routes.
#### [NEW] `internal/api/print_test.go`
Integration tests for HTTP endpoints, PDF responses, and authorization checks.

---

### Component 7: Desk UI (Print Preview & Toolbar Action)
#### [MODIFY] `desk/src/lib/components/FormView.svelte`
Adds "Print" action in the document toolbar/menu.
#### [NEW] `desk/src/routes/app/[workspace]/[doctype]/[name]/print/+page.svelte`
#### [NEW] `desk/src/routes/app/[doctype]/[name]/print/+page.svelte`
Dedicated Desk Print View route with format picker, letterhead picker, language switcher, iframe preview, and "Print" / "Download PDF" buttons.

---

### Component 8: Documentation, Fixture & Validation
#### [NEW] `apps/testapp/print/test_invoice.print.ts`
Fixture print template in testapp for end-to-end testing.
#### [NEW] `docs/agent/print.md`
Public documentation for app authors explaining print templates, blocks, formatting, and PDF generation.
#### [MODIFY] `ROADMAP.md`
Updates status of OPS-01.

---

## Implementation Tasks

### Task 1: Core DocType `Letter Head` & Translations

**Files:**
- Create: `core/doctypes/letter_head/letter_head.doctype.ts`
- Modify: `core/translations/pt-BR.csv`
- Test: `internal/engine/core_test.go`

**Interfaces:**
- Produces: DocType `Letter Head` (`tab_letter_head`), accessible via `c.St.DocType("Letter Head")`.

- [ ] **Step 1: Write the DocType definition in TypeScript**

Create `core/doctypes/letter_head/letter_head.doctype.ts`:
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

- [ ] **Step 2: Update Portuguese translations**

Add translations to `core/translations/pt-BR.csv`:
```csv
Letter Head,Timbre,# core/doctypes/letter_head/letter_head.doctype.ts:5
Letter Head Name,Nome do Timbre,# core/doctypes/letter_head/letter_head.doctype.ts:11
Is Default,É Padrão,# core/doctypes/letter_head/letter_head.doctype.ts:12
Logo Image,Imagem do Logo,# core/doctypes/letter_head/letter_head.doctype.ts:15
Header HTML,HTML do Cabeçalho,# core/doctypes/letter_head/letter_head.doctype.ts:16
Footer HTML,HTML do Rodapé,# core/doctypes/letter_head/letter_head.doctype.ts:17
```

- [ ] **Step 3: Run metadata tests and check**

Run: `go test ./internal/engine -run TestCoreMetadata`
Expected: PASS

- [ ] **Step 4: Commit Task 1**

```bash
git add core/doctypes/letter_head/ core/translations/pt-BR.csv
git commit -m "feat(core): add Letter Head doctype and translations"
```

---

### Task 2: SDK Typings & JS Prelude Registration

**Files:**
- Modify: `packages/sdk/src/types.ts`
- Modify: `packages/sdk/src/index.ts`
- Modify: `internal/js/prelude.js`
- Test: `internal/engine/engine_test.go`

**Interfaces:**
- Produces: `definePrintTemplate` in `@ddcore/sdk`, `reg.printTemplates` in JS prelude, exposed via `Snapshot.PrintTemplates`.

- [ ] **Step 1: Add type definitions in `packages/sdk/src/types.ts`**

```typescript
export interface PrintBlock {
  type: string;
  [key: string]: any;
}

export interface PrintBlockBuilder {
  header(title: string, opts?: { subtitle?: string; badge?: string; badgeColor?: string }): PrintBlock;
  keyValues(pairs: [label: string, value: any][], opts?: { columns?: 2 | 3 | 4 }): PrintBlock;
  section(title?: string, blocks?: PrintBlock[]): PrintBlock;
  table(headers: string[], rows: (string | number)[][], opts?: { aligns?: ("left" | "right" | "center")[] }): PrintBlock;
  totals(rows: [label: string, value: string][]): PrintBlock;
  p(text: string): PrintBlock;
  h(level: 1 | 2 | 3 | 4, text: string): PrintBlock;
  rule(): PrintBlock;
  pageBreak(): PrintBlock;
  raw(html: string): PrintBlock;
}

export interface PrintContext {
  formatCurrency(val: any): string;
  formatDate(val: any): string;
  formatDateTime(val: any): string;
  formatNumber(val: any, decimals?: number): string;
}

export interface PrintTemplateDef<T = any> {
  name: string;
  doctype: string;
  label: string;
  body: (doc: T, b: PrintBlockBuilder, ctx: PrintContext) => PrintBlock[];
}
```

- [ ] **Step 2: Export `definePrintTemplate` in `packages/sdk/src/index.ts`**

```typescript
export function definePrintTemplate<T = any>(def: PrintTemplateDef<T>): PrintTemplateDef<T> {
  __ddcore.register("print", def);
  return def;
}
```

- [ ] **Step 3: Register `print` in `internal/js/prelude.js`**

Handle `case "print":` in `__ddcore.register`:
```javascript
case "print": {
  if (!value || typeof value.name !== "string" || !value.name.trim()) throw new DDCoreError("ValidationError", "", "Print template: name is required");
  if (!value.doctype || typeof value.doctype !== "string") throw new DDCoreError("ValidationError", "", "Print template " + value.name + ": doctype is required");
  if (typeof value.body !== "function") throw new DDCoreError("ValidationError", "", "Print template " + value.name + ": body must be a function");
  value.app = reg.app;
  value.sourceFile = reg.current;
  reg.printTemplates[value.name] = value;
  break;
}
```
Expose stripped `printTemplates` in JSON snapshot.

- [ ] **Step 4: Verify JS registry snapshot**

Run: `go test ./internal/js -v`
Expected: PASS

- [ ] **Step 5: Commit Task 2**

```bash
git add packages/sdk/ internal/js/
git commit -m "feat(sdk): add definePrintTemplate and prelude registry"
```

---

### Task 3: Print Block Vocabulary & Standard DocType Template

**Files:**
- Create: `internal/print/blocks.go`
- Create: `internal/print/standard.go`
- Create: `internal/print/render.go`
- Create: `internal/print/render_test.go`

**Interfaces:**
- Produces: `print.RenderBlocks(blocks []Block) string`, `print.StandardTemplate(d *meta.DocType, doc map[string]any, ctx Context) []Block`, `print.AssembleHTML(bodyHTML string, letterhead *LetterHead, opts Options) string`.

- [ ] **Step 1: Implement `internal/print/blocks.go`**

Define Block struct:
```go
package print

type Block struct {
    Type       string           `json:"type"`
    Title      string           `json:"title,omitempty"`
    Subtitle   string           `json:"subtitle,omitempty"`
    Badge      string           `json:"badge,omitempty"`
    BadgeColor string           `json:"badgeColor,omitempty"`
    Columns    int              `json:"columns,omitempty"`
    Pairs      [][]string       `json:"pairs,omitempty"`
    Headers    []string         `json:"headers,omitempty"`
    Rows       [][]string       `json:"rows,omitempty"`
    Aligns     []string         `json:"aligns,omitempty"`
    Text       string           `json:"text,omitempty"`
    Level      int              `json:"level,omitempty"`
    HTML       string           `json:"html,omitempty"`
    Blocks     []Block          `json:"blocks,omitempty"`
}
```
Implement `(b Block) RenderHTML() string` with strict escaping of all text fields.

- [ ] **Step 2: Implement `internal/print/standard.go`**

Generate blocks from `meta.DocType` structure:
- Header with Title and Status.
- KeyValues for document fields arranged in sections.
- Tables for child rows.
- Omit hidden and password fields.

- [ ] **Step 3: Implement `internal/print/render.go`**

Assembles full HTML page with `@page` and `@media print` CSS, Letterhead top/bottom insertion, page numbering, and UTF-8 encoding.

- [ ] **Step 4: Write unit tests in `internal/print/render_test.go`**

Test:
- Block rendering and escaping.
- Accents (`ç`, `ã`, `é`) and rune alignment.
- Standard format auto-generation.
- Page break CSS.

- [ ] **Step 5: Run tests**

Run: `go test ./internal/print -v`
Expected: PASS

- [ ] **Step 6: Commit Task 3**

```bash
git add internal/print/
git commit -m "feat(print): add print block renderer and standard format generator"
```

---

### Task 4: Engine Integration & Security Enforcement

**Files:**
- Modify: `internal/engine/engine.go`
- Create: `internal/engine/print.go`
- Create: `internal/engine/print_test.go`

**Interfaces:**
- Produces: `(c *Ctx) PrintDoc(doctype, name, format, letterhead, lang string) (string, error)`.

- [ ] **Step 1: Add `PrintTemplates` to `engine.Snapshot`**

Update `engine.go`:
```go
type PrintTemplate struct {
    Name       string `json:"name"`
    Doctype    string `json:"doctype"`
    Label      string `json:"label"`
    App        string `json:"app"`
    SourceFile string `json:"sourceFile"`
}
```

- [ ] **Step 2: Implement `(c *Ctx) PrintDoc` in `internal/engine/print.go`**

1. Validate read permission:
   ```go
   if !c.HasPermission(doctype, "read", doc) {
       return "", cerr.Permission("You do not have permission to print {0} {1}", doctype, name)
   }
   ```
2. Validate User Access Scopes (SEC-01) with `c.checkUserPermissions(d, doc)`.
3. Filter restricted fields (`permlevel > 0`), redact `Password` and `Vault` fields.
4. If `format == "" || format == "standard"`, use `print.StandardTemplate`.
5. If custom format requested, run template in goja with isolated `__ddcoreLang` and context helpers.
6. Resolve letterhead (default or specified).
7. Return assembled HTML.

- [ ] **Step 3: Write tests in `internal/engine/print_test.go`**

Test unauthorized user denial (403), user access scope denial, and restricted field omission.

- [ ] **Step 4: Run tests**

Run: `go test ./internal/engine -run TestPrint -v`
Expected: PASS

- [ ] **Step 5: Commit Task 4**

```bash
git add internal/engine/print.go internal/engine/print_test.go internal/engine/engine.go
git commit -m "feat(engine): add secured PrintDoc engine method"
```

---

### Task 5: PDF Rendering Subsystem

**Files:**
- Create: `internal/print/pdf.go`
- Create: `internal/print/pdf_test.go`

**Interfaces:**
- Produces: `print.GetPDFRenderer() PDFRenderer`, `(r *ChromeRenderer) RenderPDF(ctx, html, opts) ([]byte, error)`.

- [ ] **Step 1: Implement `internal/print/pdf.go`**

Define `PDFRenderer` interface and adapters:
- `ChromeRenderer`: Discovers Chromium/Chrome, writes HTML to temp file, runs CLI with 20s timeout, reads PDF bytes, cleans temp files.
- `GotenbergRenderer`: Sends POST to Gotenberg conversion URL.
- `CommandRenderer`: Executes custom CLI command with `{in}` and `{out}` placeholders.
- Concurrency limiter semaphore (`make(chan struct{}, 4)`).

- [ ] **Step 2: Write tests in `internal/print/pdf_test.go`**

Test mock command adapter, timeout handling, and concurrency semaphore.

- [ ] **Step 3: Run tests**

Run: `go test ./internal/print -run TestPDF -v`
Expected: PASS

- [ ] **Step 4: Commit Task 5**

```bash
git add internal/print/pdf.go internal/print/pdf_test.go
git commit -m "feat(print): add pluggable PDF rendering engine"
```

---

### Task 6: HTTP API Endpoints

**Files:**
- Create: `internal/api/print.go`
- Modify: `internal/api/api.go`
- Create: `internal/api/print_test.go`

**Interfaces:**
- Produces: `/api/print/formats/{doctype}`, `/api/print/{doctype}/{name}`, `/api/print/{doctype}/{name}/pdf`, `/api/letterheads`.

- [ ] **Step 1: Implement handlers in `internal/api/print.go`**

- `s.listPrintFormats`: returns standard format + registered app templates for the doctype.
- `s.renderPrintDoc`: calls `c.PrintDoc`, returns HTML (or raw HTML if `raw=1`).
- `s.downloadDocPDF`: calls `c.PrintDoc`, passes to `PDFRenderer`, streams PDF bytes with proper headers.
- `s.listLetterheads`: queries active `tab_letter_head` records.

- [ ] **Step 2: Mount routes in `internal/api/api.go`**

Register routes inside `r.Group(func(r chi.Router) { r.Use(s.requireLogin) ... })`.

- [ ] **Step 3: Write tests in `internal/api/print_test.go`**

Verify endpoints, query parameters, permissions, and PDF headers.

- [ ] **Step 4: Run API tests**

Run: `go test ./internal/api -run TestPrintAPI -v`
Expected: PASS

- [ ] **Step 5: Commit Task 6**

```bash
git add internal/api/print.go internal/api/api.go internal/api/print_test.go
git commit -m "feat(api): add print preview and PDF HTTP endpoints"
```

---

### Task 7: Desk UI — Print Preview View & Form Action

**Files:**
- Modify: `desk/src/lib/components/FormView.svelte`
- Create: `desk/src/routes/app/[workspace]/[doctype]/[name]/print/+page.svelte`
- Create: `desk/src/routes/app/[doctype]/[name]/print/+page.svelte`

- [ ] **Step 1: Add "Print" button/action in `FormView.svelte`**

Add "Print" option in the form action dropdown linking to `${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}/print`.

- [ ] **Step 2: Build the Print View page component**

Includes:
- Back button.
- Format selector (`<select>`).
- Letterhead selector (`<select>`).
- Language selector (`<select>`).
- Print button (`iframe.contentWindow.print()`).
- Download PDF button (links to `/api/print/.../pdf`).
- Isolated preview iframe.

- [ ] **Step 3: Verify Desk build and types**

Run: `cd desk && npm run check && npm run test`
Expected: PASS

- [ ] **Step 4: Commit Task 7**

```bash
git add desk/
git commit -m "feat(desk): add dedicated print preview view and form action"
```

---

### Task 8: Documentation, Test App Fixture & Final Validation

**Files:**
- Create: `apps/testapp/print/test_invoice.print.ts`
- Create: `docs/agent/print.md`
- Modify: `ROADMAP.md`

- [ ] **Step 1: Create fixture template in `apps/testapp`**

Define a representative test invoice print template exercising blocks, tables, and currency formatting.

- [ ] **Step 2: Write agent documentation in `docs/agent/print.md`**

Document `definePrintTemplate`, block vocabulary, Letter Head usage, and PDF configuration.

- [ ] **Step 3: Update `ROADMAP.md`**

Mark OPS-01 as completed in `ROADMAP.md`.

- [ ] **Step 4: Run complete test and translation check**

Run: `make test && make check`
Expected: ALL PASS with 0 missing translations and 0 type errors.

- [ ] **Step 5: Commit Task 8**

```bash
git add apps/testapp/print/ docs/agent/print.md ROADMAP.md
git commit -m "docs: add print documentation, testapp fixture, and update roadmap"
```

---

## Verification Plan

### Automated Tests
1. Unit and block rendering tests:
   `go test ./internal/print/... -v`
2. Engine and security tests:
   `go test ./internal/engine/... -run TestPrint -v`
3. HTTP API integration tests:
   `go test ./internal/api/... -run TestPrintAPI -v`
4. Desk unit tests:
   `cd desk && npm run test`
5. Full repository check:
   `make test && make check`

### Manual Verification
1. Open Desk in browser (`http://localhost:8090`).
2. Navigate to a document form in `apps/testapp`.
3. Click "Print" from the form menu.
4. Verify the print preview appears in the sandboxed iframe with accurate margins and styling.
5. Change the format to custom and verify live re-rendering.
6. Click "Print" and verify the browser's native print preview dialog opens.
7. Click "Download PDF" and verify the generated PDF file downloads cleanly.
