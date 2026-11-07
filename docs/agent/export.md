# Exporting data

Taking a whole DocType out — every row a filter matches, its child tables and
its attachments — rather than the page a screen happens to be showing. It is
the extraction side of a migration, and the export a user asks for from a list.
Server code that builds a spreadsheet of its own rows — to mail it, say — uses
[`ddcore.spreadsheet.file`](#from-server-code-ddcorespreadsheetfile), with the
same cell rules.

## The permission

`export` in a DocType's `permissions` is what allows the rows to leave as a
file. It is checked on the server, separately from `read`:

```ts
permissions: [
  { role: "Manager",     read: true, write: true, export: true },
  { role: "Contributor", read: true },   // may read the list, may not export it
]
```

A role with `read` and without `export` gets a 403 from `/api/export` and no
export button in the desk. A child DocType follows its parent's `read`, the way
it does everywhere else.

## What comes out, and what never does

Every column of the DocType, in a stable order: `id`, the fields as the app
declared them, then `owner`, `creation`, `modified`, `modified_by` and
`docstatus`. The audit columns are there because a reconciliation needs them.

Two things never leave, whatever is asked for:

- every field of fieldtype `Password`;
- the core's credential columns (`User.password_hash`, `API Key.secret_hash`).

Asking for one by name is a validation error, not a silent omission.

A `Text Editor` column carries its stored HTML, unchanged, which is what makes
an export a faithful copy; a consumer that wants plain text strips it. A
`Markdown Editor` carries its source, a `Duration` or `Rating` its integer,
a `Signature` its PNG data URL and a `Geolocation` its GeoJSON
FeatureCollection. (A grid's CSV/XLSX export in the desk says "Signed" for a
signature, and a place's summary — `lat, lon` or "2 points, 1 polygon".)

A field above the exporting user's permission level (`permlevel`) is left out of the
default columns, out of child tables and, for a file held by that field, out of the
attachments. Asking for it by name is a `PermissionError`. The CLI exports as
Admin unless `--user` says otherwise. See `field-permissions`.

Row-level authorisation is the same as a list's: `ifOwner` and the controller's
`permissionQuery` restrict an export exactly as they restrict `getList`.

## HTTP

```
GET /api/export/<DocType>?format=csv|ndjson|xlsx&filters=[...]&or_filters=[...]
                         &fields=[...]&children=1&attachments=1&sep=;&limit=N
```

The response is a download (`Content-Disposition: attachment`), streamed as the
rows come out of the database, inside one `REPEATABLE READ` transaction so every
page of the walk sees the same instant.

- `X-DDCore-Export-Count` is how many rows the file should hold. Check it: once
  a `200` has been sent there is no status code left to report a failure with,
  so a short file is otherwise indistinguishable from a complete one.
- In NDJSON the last line is `{"_manifest": {…}}`. Its absence means the stream
  was cut short.
- `format=csv` with `children=1` is refused: one CSV cannot hold a parent and
  its child tables. Use `format=ndjson`, or the CLI, which writes one file per
  table. `format=xlsx` refuses `children=1` and `attachments=1` for the same
  reason: the response is one workbook.
- Above `exportMaxRows` in `ddcore.json` (default 100000) the request is
  refused and points at the CLI. An explicit `limit` is the caller accepting a
  sample, and is allowed; the manifest then carries `"truncated": true`.
- A CSV export loads back through [Data Import](data-import.md) in update mode
  unchanged: it carries `id` and `modified`, and the derived columns are
  ignored on the way in.
- `fields` picks the columns and their order. It may leave out `id` (a list
  exporting the columns on screen): the walk still pages on it, and the file
  does not carry it — nor can it then be imported back as an update.

## CLI

```bash
ddcore export <DocType> [--filters '<json>'] [--format ndjson|csv|xlsx] [--out DIR]
              [--children] [--attachments] [--fields a,b] [--sep ,]
              [--batch 500] [--user <email>] [--all]
```

`--all` walks every DocType of the site, skipping child tables (they travel
inside their parent) and Singles. `--user` runs the export as that user, so the
permission model is exercised rather than assumed — with `--all`, a DocType that
user may not export is recorded in the manifest's `skipped` instead of ending
the run. There is no row cap here.

```
<out>/<DocType>.ndjson                   (or <DocType>.csv + <DocType>.<field>.csv per child table;
                                          .xlsx the same way, one workbook per table)
<out>/<DocType>.files.csv                (the attachment manifest, CSV only)
<out>/files/public/<file>                (the attachment bytes, with --attachments)
<out>/files/private/<file>               (a private file keeps its own prefix)
<out>/manifest.json                      (counts, checksums, filters, user, versions, timings)
```

The bytes sit under their storage key rather than their base name, so a public
and a private file that happen to share a name do not overwrite each other;
`manifest.json` records the layout as `"exportFormat": 2`.

`manifest.json` is what makes a load reconcilable: it carries the sha256 of
every file written and of every attachment, so two runs of the same export can
be compared and what was loaded downstream can be checked against what was
taken. It also records the core version and an `apps` map of each loaded app's
declared `version` — the value only, never the `ddcore` range it accepts, so a
manifest says what produced the export and not what could read it back (see
[conventions.md](conventions.md)).

## Formats

**NDJSON** is the reconcilable one: one document per line, child tables nested
under their fieldname, values identical to what `/api/resource` returns. With
`--attachments`, each document carries its `_files` manifest.

**CSV** is for a person with a spreadsheet. It follows the rules the desk's own
CSV follows: a UTF-8 BOM (without it Excel reads `Endereço` as `EndereÃ§o`),
CRLF, RFC 4180 quoting, and a separator that is `,` unless the caller asks for
`;` — where the decimal mark is a comma, the column separator cannot be one too.

A value renders the same in both: a `Check` is `true`, never `1`, and a `JSON`
column keeps its compact JSON. That is deliberate — the two files get compared
side by side.

Child tables in CSV are separate files keyed back by `parent`, `parenttype`,
`parentfield` and `idx`.

**XLSX** is the CSV for a person who works in Excel: the same columns, with
the fieldnames as the header, in a workbook whose cells keep their types — so
nobody picks a separator, an encoding or a column type on the way in, and a
pt-BR Excel does not misread `10.5` or `03/01`:

| Column | Cell |
|---|---|
| `Currency` | a number, shown with the site's currency decimals |
| `Int`, `Float`, `Percent`, `Duration`, `Rating`, `docstatus`, `idx` | a number |
| `Check` | a boolean |
| `Date`, `Month` | a date |
| `Datetime`, `creation` | a date and time on the **site's** clock (`ddcore.json:timezone`) — a cell has no timezone |
| `Time` | a time of day |
| `modified` | the CSV's text, the exact instant: Data Import compares it to refuse a row changed since the export, and a day number would not match |
| anything else | the CSV's text: a `Select` its stored value, a `Link` the id |

The dates use Excel's built-in formats, so each reader sees their own date
order. A value that does not parse as its type is kept as text rather than
dropped, a cell holds at most 32767 characters (Excel's limit), and a sheet at
most 1048575 rows — the CLI fails past that rather than write a workbook Excel
cannot open. An XLSX loads back through [Data Import](data-import.md) like the
CSV does: a numeric cell in a Date, Datetime or Time column is read as a date
on the site's clock.

The workbook is streamed: the server holds one row at a time, whatever the
size. It is also written when no row matches — the header and nothing else.

## From server code: `ddcore.spreadsheet.file`

An app that needs a spreadsheet of its own making — a purchase order's grid of
product × colour × size × store to mail the supplier — writes it with
`ddcore.spreadsheet.file`. It takes columns and rows rather than a DocType, and
returns a `File`, the way [`ddcore.print.pdf`](print.md#from-server-code) does:
private, attached to the document, on the current transaction, and what
`sendMail`'s `attach` takes.

```ts
// apps/purchasing/services/po_mail.ts (server, synchronous)
const xlsx = ddcore.spreadsheet.file({
  doctype: "Purchase Order", id: po.id,          // the owner: must exist, needs write
  filename: `${po.id}.xlsx`,                     // default <doctype>-<id>.xlsx
  sheets: [
    { name: "Order", columns: [{ label: _("Order") }, { label: _("Supplier") }], rows: [[po.id, po.supplier]] },
    {
      name: "Grid",
      columns: [
        { label: _("Reference"), fieldtype: "Data" },
        { label: _("Pieces"), fieldtype: "Int" },
        { label: _("Unit Cost"), fieldtype: "Currency" },
        { label: _("Delivery Date"), fieldtype: "Date" },
      ],
      rows: po.items.map((i) => [i.reference, i.pieces, i.unit_cost, i.delivery_date]),
    },
  ],
});
ddcore.sendMail({ to, template: "purchasing.purchase_order_supplier", attach: [pdf.id, xlsx.id], key });
```

- **`format`** is `"xlsx"` (the default) or `"csv"`, which a `filename` ending in
  `.csv` also picks. The extension is added to a `filename` without one, and a
  `filename` ending in the other format's is refused.
- **The cells** follow the [XLSX table above](#formats): a column's `fieldtype`
  types its cells — a `Currency` is a number with the site's decimals, a `Date`
  (`"YYYY-MM-DD"`) a date, a `Datetime` (ISO) a date and time on the site's
  clock, a `Check` a boolean (`true` or `1`). A column with no `fieldtype` keeps
  each value's own type: a number stays a number. A `fieldtype` that holds no
  value (`Table`, `Section Break`…) or does not exist is refused.
- **The rows** are arrays in the columns' order. A short row is padded with
  empty cells; a row longer than the columns is refused.
- **The header** is the labels as given: translate them with `_()`, in the
  language the reader wants — the supplier's, not necessarily the user's.
- **Excel's limits throw** instead of trimming: a cell past 32767 characters
  (naming the cell, `B12`), a sheet past 1048575 rows, a sheet name past 31
  characters or with one of `[ ] : * ? / \`, an apostrophe or a space at either
  end, `History`, or two sheets of one name (Excel ignores case). An unnamed
  sheet is `Sheet1`, `Sheet2`…
- **A CSV** is the export's CSV: the UTF-8 BOM, CRLF, a `Check` as
  `true`/`false`, a number with a `.` decimal mark. It holds exactly one sheet
  and has no cell limit. `sep` (CSV only, one character) sets the separator —
  `";"` for readers whose decimal mark is a comma.

Attaching is writing, as for any upload: the user needs write on the document,
and the spreadsheet is then readable by everyone who reads it. A job or a hook
that runs for a user who may not write passes `ignorePermissions: true`. The
File is written on the caller's transaction, so a rollback takes the row and the
bytes back out. The workbook is built in memory, which suits a mail attachment
(`DDCORE_MAIL_MAX_ATTACHMENT`, 10 MiB by default); a whole DocType is the
export's job.

## Attachments

`--attachments` (or `attachments=1`) lists the `File` rows attached to each
exported document with their `sha256`, and the CLI copies the bytes into
`<out>/files/`. A file attached to several documents is copied once.

A row whose bytes the store cannot find is marked `"missing": true` rather than
skipped: that is a finding for the reconciliation, not a reason to abort.

An attachment on a document you may read is yours to export even when someone
else uploaded it — the same rule `/private/files` applies.

## Loading it back

An export directory is what [`import`](import.md) reads: `ddcore import run <dir>`
puts it into another site with the ids, owners and timestamps intact, and
`ddcore import reconcile <dir>` checks the two agree.

## The desk

A list's download button asks what to export: the page on screen (built in the
browser, with Link titles resolved) or everything the filters match (streamed
by the server), as CSV or XLSX — and everything matching as NDJSON too. The
filters sent are the ones the list is showing. A CSV or XLSX holds the id,
always and first — hidden or not, so the file loads back through Data Import in
update mode — then the columns on screen, in their order: **Modified** when it
is shown, and whatever the user changed with the Columns menu. An NDJSON export
is a whole copy of each row, every column, for an import.

The page on screen is built in the browser with the columns' labels as the
header and a Link's title in its cell; its XLSX types the cells as the server's
does (a Datetime on the site's clock). Everything matching comes from the
server, with fieldnames as the header and a Link's id.

A report and a form grid (a `Table` or `Report` field with `gridExport: true`)
export in the browser, as CSV or XLSX: the rows already on screen, in their
order, or only the selected ones. The same `export` permission gates the
button. See `fieldtypes`, "Form grids".
