# Data Import (CSV and XLSX)

Loading rows from a spreadsheet: a CSV or the first sheet of an Excel `.xlsx`
file, each row creating or updating one document. The Desk offers it from a
list's toolbar; the same thing is an HTTP endpoint.

This is **not** [`ddcore import`](import.md). That command loads another
site's history as it was: ids, owners and timestamps kept, no hook and no side
effect, administrators only. A spreadsheet is new business data. Each row is
an ordinary insert or save **as the person who uploaded it**, so everything a
form goes through applies to it:

- roles and User Permission scopes (`scopes`);
- field levels (`field-permissions`);
- controller hooks and the id rule;
- workflows;
- webhooks, notifications and Version.

## The permission

`import` in a DocType's permission rows lets a role load files into it:

```ts
permissions: [
  { role: "Manager", read: true, write: true, create: true, import: true },
]
```

- **Creating** records also needs `create`.
- **Updating** them also needs `write`.

A role with `import` alone can do neither. Admin passes, as it does everywhere.

Some DocTypes are refused whatever the roles say:

- child tables, which travel inside their parent;
- Singles;
- `Audit Event`, `Error Log`, `Email Delivery`, `Webhook Delivery`, `Webhook` and `API Key`.

## Two modes

**Create new records** (`mode=insert`, the default). Each row is one new
document.

- A blank cell keeps the field's default.
- An `ID` column is honoured only when the DocType leaves the id to whoever
  creates the document, that is, a random id or a prompt. Under a series, a
  field or a format, the column is ignored and the DocType's rule names the
  record.

**Update existing records** (`mode=update`). An `ID` column is required and
names the record each row changes.

- Only the columns the file carries change.
- A blank cell **clears** the field.
- A `modified` column, which every export has, is checked: a row whose record
  changed on the site after the file was exported is refused with
  `TimestampMismatchError` rather than overwriting that change.
- A submitted document accepts only its `allowOnSubmit` fields.

There is no upsert: the mode is chosen, never guessed from the data.

`docstatus` is never read. Everything lands as a draft, and a submittable
document is submitted afterwards, as with a form.

## Columns

The first non-blank row names the columns. Each header is matched, ignoring
case and surrounding spaces, against these in order; the first match wins:

1. a fieldname (`start_date`);
2. `id`, `ID` or the DocType's `idLabel`;
3. the English label (`Start date`);
4. the label in the request's language (`Data de início`).

A label two fields share matches neither. The column is reported and the
fieldname has to be the header.

A column that matches nothing is **ignored and reported**, never refused. That
is what lets the file of failed rows the Desk hands back, which has an extra
`Error` column, be fixed and uploaded again.

Some columns are ignored even though they match, each with its reason in the
result:

- standard columns (`owner`, `creation`, `docstatus`, and `modified` when
  inserting);
- `readOnly` and `fetchFrom` fields, so re-importing an export does not
  overwrite derived values;
- the workflow's state field;
- `Password` and `Vault` fields;
- `Attach` and `Attach Image` fields;
- child tables, `Table MultiSelect` included;
- fields above the user's writable permission level.

The request can override the matching with `columns`, a JSON object mapping a
header to a fieldname, or to `""` to ignore that column. The same rules apply
to the fieldname it names.

## Cell values

A cell is text a person typed. The importer reads each one strictly according
to its field, before the save validates it again. The save's own coercion is
forgiving on purpose, for typed JSON from the API, and would read `abc` as `0`.

| Field | Accepted |
| --- | --- |
| Int, Float, Currency, Percent, Duration, Rating | A number written with the request's `decimal` separator. The other separator is read as a thousands separator, so `1.234,56` with `decimal=,`. Spaces and a trailing `%` on a Percent are dropped. An Int, Duration or Rating must be whole. |
| Check | `1`/`0`, `true`/`false`, `yes`/`no`, `y`/`n`, `x`, and "Yes"/"No" in the request's language. Anything else is an error. |
| Date | ISO `2026-12-31`, or three numbers in the request's `date_order` (`dmy`, `mdy`, `ymd`) separated by `/`, `.` or `-`. A four-digit first number is always the year. A two-digit year below 30 is 20xx. `31/02` is an error, not the 3rd of March. |
| Datetime | Any ISO form, or a date as above followed by `HH:MM[:SS]` in the site's timezone. |
| Select | The stored value, or the label the user sees in their language, in any case. |
| Link | The target's id, or its `titleField` value when that names exactly one record the user can read. Two matches are an error: the file has to use the id. |
| anything else | The trimmed text, as is. A code like `007` stays `007`. |

The Desk dialog sends the user's locale for `decimal` and `date_order`. It
also offers both as selects, because a file often comes from someone whose
spreadsheet writes `15/02/2026` or `1.234,56` differently from the person
uploading it.

In an XLSX, a numeric cell in a Date, Datetime or Time column is an Excel date
serial (both the 1900 and 1904 systems). A formula contributes the value Excel
cached when the file was saved.

## Dry run, then import

```
POST /api/data-import/<DocType>        multipart/form-data
  file        the .csv / .txt / .xlsx (detected by content, not by name)
  mode        insert | update
  dry_run     1 to check and write nothing
  sep         CSV separator; omitted, it is read from the header line (, ; or tab)
  decimal     . or ,
  date_order  dmy | mdy | ymd
  columns     optional JSON {"<header>": "<fieldname>" | ""}
```

A **dry run** writes every row in one transaction, with a savepoint per row,
and rolls it all back. Row *n* sees rows 1 to *n*−1, so a duplicate inside the
file, or a link to a record the file creates further up, fails exactly as it
would in the real run. Series counters roll back with it.

It **runs the controller hooks**, and that is how it predicts the real run.
Webhooks, mail and jobs are queued inside the transaction and vanish with it,
but a hook that calls `ddcore.http` really calls out.

The **real run** commits every row in its own transaction. A bad row costs
that row only, and each good row's after-commit effects fire as it commits.
Stopping halfway (the client going away) leaves the committed rows in place.

The response lists every row:

```json
{ "data": {
  "doctype": "Project", "mode": "insert", "dryRun": true,
  "file": { "name": "p.xlsx", "format": "xlsx", "sha256": "…" },
  "headers": ["Code", "Title", "Assignee"],
  "columns": [{ "index": 0, "header": "Code", "fieldname": "code", "label": "Code", "status": "mapped" }],
  "fields":  [{ "fieldname": "code", "label": "Code", "reqd": true }],
  "counts":  { "rows": 3, "inserted": 2, "updated": 0, "errors": 1 },
  "rows": [
    { "row": 2, "status": "inserted", "id": "P-100" },
    { "row": 3, "status": "error", "type": "LinkExistsError", "message": "…", "cells": ["P-101", "Beta", "ghost@x.com"] }
  ] } }
```

- `row` is the number the spreadsheet shows, with the header as row 1.
- Messages come in the request's language.
- `cells` are sent only for failed rows.

Only problems with the upload as a whole are HTTP errors:

- the permission (403);
- an unreadable, empty or `.xls` file, a file with no usable column, or an
  update without an ID column (417);
- maintenance mode (503).

## The template

`GET /api/data-import/<DocType>/template?sep=;` is the header row of a file
that creates records:

- every field a column may fill, labelled in the request's language;
- the fieldname wherever a label would be ambiguous;
- an `id` column only when inserts may set one;
- a BOM and CRLF, as an export has.

To update, start from the list's export ("All rows matching the filters as
CSV"). It carries `id` and `modified`, and imports back unchanged: a file
exported and re-imported in update mode reports every row updated and none
failed.

## Limits

- `importMaxRows` in `ddcore.json` caps the data rows of one file (default
  5000). Every row is written while the request waits, and the cap is what
  keeps an upload inside a reverse proxy's timeout. The file itself is capped
  at 10 MB.
- The upload is not stored: the Desk posts the same file for the dry run and
  for the real run.
- A real run records one `data.import` event in `Audit Event` (see `audit`)
  with the file name, its sha256, the mode and the counts. The documents
  themselves get their Version entries like any save. A dry run records
  nothing.

## Not covered

- No child table rows: a spreadsheet has no layout the CSV export shares for
  them.
- No `.xls` and no sheet other than the first.
- No submit-on-import.
- No background job or progress bar, so large loads belong to `ddcore import`
  or to several files.
- No CLI or MCP tool for spreadsheets; agents have `insert_doc` and `update_doc`.
- A dry run of many rows is one transaction holding a savepoint per row.
