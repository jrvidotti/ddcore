# Exporting data

Taking a whole DocType out — every row a filter matches, its child tables and
its attachments — rather than the page a screen happens to be showing. It is
the extraction side of a migration, and the export a user asks for from a list.

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

A field above the exporting user's permission level (`permlevel`) is left out of the
default columns, out of child tables and, for a file held by that field, out of the
attachments. Asking for it by name is a `PermissionError`. The CLI exports as
Admin unless `--user` says otherwise. See `field-permissions`.

Row-level authorisation is the same as a list's: `ifOwner` and the controller's
`permissionQuery` restrict an export exactly as they restrict `getList`.

## HTTP

```
GET /api/export/<DocType>?format=csv|ndjson&filters=[...]&or_filters=[...]
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
  table.
- Above `exportMaxRows` in `ddcore.json` (default 100000) the request is
  refused and points at the CLI. An explicit `limit` is the caller accepting a
  sample, and is allowed; the manifest then carries `"truncated": true`.

## CLI

```bash
ddcore export <DocType> [--filters '<json>'] [--format ndjson|csv] [--out DIR]
              [--children] [--attachments] [--fields a,b] [--sep ,]
              [--batch 500] [--user <email>] [--all]
```

`--all` walks every DocType of the site, skipping child tables (they travel
inside their parent) and Singles. `--user` runs the export as that user, so the
permission model is exercised rather than assumed — with `--all`, a DocType that
user may not export is recorded in the manifest's `skipped` instead of ending
the run. There is no row cap here.

```
<out>/<DocType>.ndjson                   (or <DocType>.csv + <DocType>.<field>.csv per child table)
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
by the server). The filters sent are the ones the list is showing.
