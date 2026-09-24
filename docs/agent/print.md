# Print templates and PDF

A document prints as a self-contained HTML page, either in the standard layout
built from its DocType or through a template an app declares. The same HTML is
what the PDF renderer converts, so a preview and its PDF come from one render.

## The standard layout

Every DocType has a `standard` format, with no code.

- The header is the DocType's label, followed by `: <value>` when the DocType
  has a `titleField` with a value, and the document id as the subtitle. A
  submittable DocType gets a badge: Draft, Submitted or Cancelled. Other
  DocTypes get none.
- Fields go into a fixed two-column grid, in declaration order. A Section Break
  with a label starts a heading. Column Breaks and Tab Breaks are skipped, so the
  form's columns and tabs do not carry over. Empty values, `Hidden` fields, and
  Password and Vault fields are left out.
- A Table field prints as a heading and a table. Its columns are the child
  DocType's visible fields: the first six, plus any later field marked
  `inListView`. Hidden, Password and Vault fields are left out here too.
- A Table MultiSelect prints as one line of the grid: its values, joined with
  `, `.
- Currency prints in the site currency, with the grouping and decimal mark of the
  print language: with a USD site, `$ 50,000.50` in `en` and `US$ 50.000,50` in
  `pt-BR`. Date prints `dd/mm/yyyy` for a `pt*` language and ISO otherwise;
  Datetime is shown in the site timezone. Percent follows the language's
  separators, a Check prints `✓` when set, and a Select value is translated.

## A template

A template lives in `print/<name>.print.ts`. Nothing registers it; the file is
enough.

```ts
import { definePrintTemplate, _ } from "@ddcore/sdk";

export default definePrintTemplate({
  name: "sales.invoice_official",
  doctype: "Invoice",
  label: "Official Invoice",
  body: (doc, b, ctx) => [
    b.header(doc.id, { subtitle: _("Invoice"), badge: _("Paid"), badgeColor: "green" }),
    b.columns([
      [b.h(3, _("Billed To")), b.p(doc.customer_name)],
      [b.keyValues([[_("Posting Date"), ctx.formatDate(doc.posting_date)]])],
    ]),
    b.rule(),
    b.table(
      [_("Item"), _("Qty"), _("Amount")],
      (doc.items || []).map((i: any) => [i.description, i.qty, ctx.formatCurrency(i.amount)]),
      { aligns: ["left", "right", "right"] },
    ),
    b.totals([[_("Grand Total"), ctx.formatCurrency(doc.grand_total)]]),
  ],
});
```

`name` is unique across every installed app, so prefix it with the app's own;
declaring the same name twice is a load error. `doctype` is the one DocType the
template prints: asking for it on another DocType is a `ValidationError`.
`label` is what the format list shows, translated; without one the list shows
`name`. The label is a catalogue key, collected by `ddcore i18n extract`.

`body` runs on the server, synchronously, in the print language, so a literal
`_("…")` in it comes out translated. `doc` is a `Document` built from the
document's values after redaction (below), and `ddcore.*` is available and runs
as the user who asked for the print.

## Blocks

`body` returns blocks. Text is escaped when rendered; `richText` and `markdown`
render markup, cleaned by the allowlist, and only `raw`/`html` is neither
escaped nor cleaned.

- `b.header(title, { subtitle, badge, badgeColor })` — a title line with an
  optional subtitle and badge. `badgeColor` is `green`, `red`, `blue`, `orange`
  or `gray` (the default). There is no logo option; a logo belongs to the Letter
  Head.
- `b.section(title, blocks)` — a titled group of blocks.
- `b.keyValues(pairs, { columns: 2 | 3 | 4 })` — label and value pairs in a grid,
  two columns unless told otherwise.
- `b.table(headers, rows, { aligns })` — a table; `aligns` holds `left`,
  `right` or `center` per column. The stylesheet asks for the header row to
  repeat on each printed page and for a row not to split across pages.
- `b.totals(rows)` — right-aligned label and value pairs, the last row emphasized.
- `b.columns(cells)` — side-by-side columns of equal width; each cell is a list
  of blocks.
- `b.p(text)` — a paragraph.
- `b.h(level, text)` — a heading; `b.h1`, `b.h2` and `b.h3` are shorthands.
- `b.rule()` — a horizontal line; `b.divider()` is the same block.
- `b.pageBreak()` — starts a new page.
- `b.richText(html, title?)` — a `Text Editor` value, rendered as the markup it
  is and cleaned by the server's allowlist first, so a template cannot print
  what a document may not hold. `title` labels it like a field.
- `b.markdown(source, title?)` — a `Markdown Editor` source, rendered and
  cleaned the same way.
- `b.pre(text, title?)` — preformatted text, escaped, keeping its whitespace: a
  `Code` field.
- `b.html(markup)` / `b.raw(markup)` — markup inserted as written, unescaped.

A block type the renderer does not know renders as nothing.

`ctx` formats values in the print language:

- `ctx.formatCurrency(value)` — two decimals with the language's separators
  (`1.500,00` in `pt-BR`), with no currency symbol. `ddcore.utils.formatCurrency`
  adds the site currency's symbol.
- `ctx.formatDate(value)` — `dd/mm/yyyy` for a `pt*` language, ISO otherwise.
- `ctx.formatDateTime(value)` — the value as stored.
- `ctx.formatNumber(value, decimals = 2)`.

## What a print can read

Printing loads the document the way a read does: the user needs `read`
permission on it, and user access scopes apply, so a document outside the user's
scopes is refused like any other read.

Before the standard layout or a template sees the document, Password and Vault
fields are removed, and so is every field above the user's permission level
(`permlevel`), in the parent and in child rows (a child row is judged by the
parent's access); the standard layout also drops their labels and child-table
columns. See `field-permissions`. Nothing else is removed. The standard layout
also skips `Hidden` fields, but a custom template receives them, so a template
must not print a hidden field it should not show. A template's own `ddcore.db`
calls are server code and are not filtered.

## Letter Head

The core `Letter Head` DocType carries the branding a print wraps around its
content. Its fields are `letter_head_name`, `is_default`, `disabled`, `align`
(`Left`, `Center` or `Right`), `image` (an attached logo) and `header_html` and
`footer_html`. System Manager edits them; every user can read them.

Only one Letter Head is the default: saving an enabled one with `is_default` set
clears the flag on the others. Saving a disabled one as the default does not
touch the others' flag — a disabled Letter Head never becomes, or clears, the
default, since prints must not silently lose their letterhead. Should several
defaults exist anyway (written by SQL, say), the most recently modified enabled
one is used.

The header is the logo, then `header_html`; the footer is `footer_html`; both
follow `align`. `header_html` and `footer_html` are inserted unescaped, as
written. The header is placed once before the content and the footer once after
it: they are not repeated on each page, and there are no page numbers.

## HTTP

| Endpoint | Returns |
| --- | --- |
| `GET /api/print/formats/{doctype}` | `[{name, label, default}]`, `standard` first |
| `GET /api/letterheads` | enabled Letter Heads, `[{id, is_default, disabled}]`, default first |
| `GET /api/print/{doctype}/{id}` | the HTML |
| `GET /api/print/{doctype}/{id}/pdf` | the PDF |

The two lists require a signed-in user, and the format list also requires
`read` permission on the DocType. The two print endpoints check `read`
permission on the document, as described above.

The two print endpoints take the same query parameters:

- `format` — `standard` (the default) or a template name.
- `letterhead` — a Letter Head id, `none` for no Letter Head, or empty for the
  default. An id that does not exist or is disabled is a `ValidationError`.
- `lang` — the print language; without it, the request's language.
- `page_format` — `A4` (the default) or `Letter`, in any case. Anything else is a
  `ValidationError`.
- `landscape` — `1` or `true` for landscape.

`page_format` and `landscape` are written into the HTML's `@page` rule, which is
what the browser's print dialog and Chrome follow; a PDF command follows them if
the tool reads `@page`. The PDF endpoint also takes `download`: `1` or `true`
sends the file as an attachment named `<doctype>-<id>.pdf`; otherwise it is
inline.

## PDF

The renderer is chosen on the first PDF request and kept for the life of the
process, so a change to the environment or a browser installed later needs a
restart. In order:

- `DDCORE_GOTENBERG_URL` set: the HTML is posted to Gotenberg's
  `/forms/chromium/convert/html`, with the paper size and orientation as form
  fields. No margins are sent, so Gotenberg applies its own. The request times
  out after 60 seconds. At most 5 renders run at once.
- `DDCORE_PDF_COMMAND` set: the command runs through `sh -c` (`cmd.exe /c` on
  Windows), with `{in}` and `{out}` replaced by the paths of a temporary HTML
  file and the PDF to write, for example `weasyprint {in} {out}`. At most 3
  renders run at once.
- Otherwise, a Chrome, Chromium, Brave or Edge binary found in the usual install
  locations or on `PATH` runs headless with `--print-to-pdf`. At most 3 renders
  run at once.

A request waits for a free slot for as long as the request lasts. The limits are
fixed.

When no renderer is available, or Gotenberg cannot be reached or does not start
answering within the 60 seconds, the PDF endpoint answers 503 `UnavailableError`. Any other renderer failure — Gotenberg answering
with an error, Chrome or the command failing — is a 500.

## Desk

A saved document's form has a Print button, also in its actions menu, that
opens `/app/<workspace>/<doctype>/<id>/print`. The page shows the HTML in an iframe,
with pickers for the format, the Letter Head (preselecting the default) and the
language. The language picker offers a fixed list: Português (Brasil), English
and Español. The desk does not send `page_format` or `landscape`.

Print calls the iframe's `contentWindow.print()`, falling back to
`window.print()`. Download PDF fetches the PDF endpoint; on a 503 it shows a hint
to use the browser's print instead, and on any other error it shows the error's
message.
