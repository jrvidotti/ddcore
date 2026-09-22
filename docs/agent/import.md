# Importing data

Loading what [`export`](export.md) wrote into a site: the same documents, with
the ids, the owners and the timestamps they already had. It is the loading side
of a migration, rehearsed as many times as it takes and run once for real.

Two properties matter more than anything else here.

**It replays nothing.** A loaded document runs no controller hook, queues no
webhook, no notification and no email, writes no Version and publishes no
realtime event. Its effects happened on the site it came from, years ago.

**It loads once.** Every line leaves a ledger row, so running the same
directory again writes nothing, and a run that stops halfway resumes at the
first line that did not commit.

## The four questions

```bash
ddcore import plan      <dir>              # what would be loaded, and what is left out
ddcore import run       <dir> --dry-run    # does it load? (one transaction, rolled back)
ddcore import run       <dir>              # load it
ddcore import reconcile <dir>              # does the site hold what the export held?
```

`ddcore import status` lists the runs, and `status <id>` reports on one with
its per-record errors.

```
--map f.json      the mapping (below)
--dry-run         validate everything and write nothing
--batch N         lines per transaction (default 500)
--only a,b        load just these DocTypes
--include a,b     load these although they are excluded by default
--resume <id>     continue a run that stopped
--max-batches N   stop after N batches, leaving the run resumable
--maintenance     pause the site for the load, and let it back in afterwards
--verify-bytes    reconcile: read every stored attachment back
--json, --report f.json
```

The same thing is an MCP tool — `import` with `action: plan|run|status|
reconcile` — where `max_batches` lets a long load advance over several calls
instead of one that times out.

## What it keeps, and what it checks

Kept as given: `id`, `owner`, `creation`, `modified`, `modified_by`,
`docstatus` (0, 1 **and 2** — history has cancelled documents, and no ordinary
write path can produce one), `amended_from`, the workflow state, and each child
row's own id. Child rows are ordered by their `idx` and renumbered densely from
there.

Checked anyway: the casts, the required fields, a Select's options, `unique`
and `uniqueKeys`, and the links. A load that fills the database with rows the
site cannot read has only moved the problem.

Not kept, because an export never carries them: `Password` and `Vault` fields,
and `User.password_hash`. Imported users have no password and get in through
`ddcore user invite`, recovery or single sign-on; the run reports every secret
field it could not bring.

After each record the load also advances `ddcore_series` past the id it wrote —
otherwise the next document created here takes an id the import just used —
and marks a date notification whose day has already passed as done, so nobody
is reminded about an invoice from two years ago.

## The order, and the links it cannot satisfy

DocTypes are loaded so that a link's target comes first: Users before Projects,
Projects before Tasks. Some links have no such order — a self link like
`amended_from`, two DocTypes that point at each other, and every Dynamic Link,
whose target is a value rather than a declaration. Those are left unchecked
during the load and verified at the end, in one query per field. What still
points at nothing is reported as `dangling`, and the run ends
`completed_with_errors`.

## A document that is already there

By default the two are compared field by field, ignoring metadata: identical is
a skip, different is a conflict recorded against that line. That default is
what lets a whole-site export land on a site `migrate` has already seeded with
its roles and its `Admin`. Per DocType the mapping can ask for `skip` (leave
what is there, whatever it is) or `error` (refuse every collision).

## Attachments

A document's `_files` manifest is loaded with it: the bytes are read from the
export, checked against the sha256 the manifest recorded, written under the
same storage key — so `file_url` still resolves and every link to the file
still works — and the `File` row keeps its own id. Bytes that do not match
their checksum fail the record; that is what the checksum is for. A file the
export itself recorded as `missing` is a note rather than a failure.

A public file whose extension a browser would execute on this origin (`.html`,
`.svg`, `.js`…) is refused. The upload handler rewrites such a name; an import
keeps the url it is given, so it has to say no instead.

## What is left out by default

| DocType | Why |
| --- | --- |
| `Audit Event` | the ledger is immutable |
| `Error Log` | a log of the other site's own errors |
| `Email Delivery`, `Webhook Delivery` | records of messages already sent |
| `Webhook` | its secret is not exported, and it would start firing |
| `API Key` | its secret is not exported, so the keys would not work |

`--include <DocType>` overrides that for everything but `Audit Event`. Singles
are never exported, and a child table travels inside its parent.

## The mapping

Somebody else's export rarely uses this site's names.

```json
{
  "ignoreUnknown": false,
  "users": { "old@acme.com": "new@acme.com" },
  "exclude": ["Legacy Log"],
  "doctypes": {
    "Sales Invoice": {
      "to": "Invoice",
      "fields": { "customer_name": "customer", "legacy_col": null },
      "set": { "company": "Acme" },
      "values": { "status": { "Rascunho": "Draft" } },
      "ids": { "SI-0001": "INV-0001" },
      "onExisting": "identical",
      "children": { "items": { "to": "lines", "fields": { "qty_": "qty" } } }
    }
  }
}
```

`fields` renames, and `null` drops. `ids` and `users` are rewritten wherever
they are mentioned — Link fields, Dynamic Links, `owner`, `modified_by` and the
reference columns — so a remapped id stays consistent.

## Reconciling

`reconcile` reads the export again and asks the site the same questions about
the ids the ledger says it loaded: the row counts, the child rows per table,
the docstatus distribution, the attachment counts, and every `Currency` and
`Percent` field summed per docstatus. The sums are compared as exact decimals,
each source value first rounded the way this site rounds it. The command exits
non-zero on any disagreement, so it can gate a cutover.

`--verify-bytes` additionally reads every stored attachment back.

## Running one

```bash
ddcore export --all --children --attachments --out /srv/export   # on the old site
ddcore import plan /srv/export                                   # on the new one
ddcore import run  /srv/export --dry-run
ddcore import run  /srv/export --maintenance
ddcore import reconcile /srv/export --verify-bytes
```

A load started from the CLI is not stopped by maintenance mode; `--maintenance`
turns it on for the duration so nothing else writes while it runs. Other
running servers keep their in-memory caches (roles, scopes, shares) until their
TTL passes, so restart them after loading users or permissions.

## Not covered

No CSV or XLSX input, and no Desk screen: the input is an NDJSON export
directory. Ids are never generated — a record without an id is an error, since
identity is the thing being preserved. Controller hooks cannot be opted into.
An interrupted run can leave attachment bytes whose record rolled back; loading
again writes the same key, and no sweep removes an orphan. A `Currency` value
has already been through a float64 in the export, and a very long dry run is
one long transaction.
