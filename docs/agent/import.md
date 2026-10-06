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
--tenant <slug>   load into this tenant (run, status, reconcile; see below)
--json, --report f.json
```

The same thing is an MCP tool — `import` with `action: plan|run|status|
reconcile` — where `max_batches` lets a long load advance over several calls
instead of one that times out, and `tenant` names the tenant to load into.

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
otherwise the next document created here takes an id the import just used, and
in a tenant it is that tenant's series that moves —
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
turns it on for the duration so nothing else writes while it runs. The users,
roles, scopes and shares it loads clear the running servers' caches when the load
commits, as a save would (see *Caching* in `scopes`), so no restart is needed.

## Into a tenant

On a site with [tenancy](tenancy.md), a load goes into the platform space unless
it names a tenant:

```bash
ddcore import run       /srv/export --tenant acme --dry-run
ddcore import run       /srv/export --tenant acme
ddcore import reconcile /srv/export --tenant acme
ddcore import status --tenant acme
```

`--tenant` may also go before the command (`ddcore --tenant acme import run …`);
naming two different tenants is an error. The tenant has to exist and be
enabled. Everything the load writes is the tenant's: the documents, the ledger
(so the same export loads into two tenants, once in each), the numbering series
and the `import.run` audit events. A run remembers its tenant: `--resume`
refuses a different one, the hint a paused run prints carries it, and `status`
lists the runs of one space — the platform's without `--tenant`.

What a tenant cannot hold is left out:

- **Shared DocTypes** are site-wide; the plan lists them as left out, "shared:
  site-wide, load it from the platform space". Load them once, without
  `--tenant`.
- **`Admin` and `Guest`** belong to the platform space. Their lines are skipped
  with a note, and reconcile does not count them.
- **An attachment whose url a file of another space already holds** fails its
  record. Storage keys carry no tenant, so writing those bytes would overwrite
  the other tenant's file. Uploads get random names, so in practice this is
  one export loaded into two tenants: the second load's attachments fail.

A user's e-mail is site-wide too: a user who already has an account in another
space fails its record as a duplicate.

This is the way to bring a customer's data into a tenant without touching the
rest of the site. `ddcore tenant adopt` instead moves *everything* in the
platform space into a tenant at once — the path for a whole site that had one
customer before it had tenancy.

## Not covered

The input is an NDJSON export directory; a spreadsheet a person fills in is
loaded by [Data Import](data-import.md) instead, as ordinary saves. Ids are never generated — a record without an id is an error, since
identity is the thing being preserved. Controller hooks cannot be opted into.
An interrupted run can leave attachment bytes whose record rolled back; loading
again writes the same key, and no sweep removes an orphan. A `Currency` value
has already been through a float64 in the export, and a very long dry run is
one long transaction.
