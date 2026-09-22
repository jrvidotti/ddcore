# Design: Resumable Import and Reconciliation (DAT-01)

This document explains **why** the importer is shaped the way it is. The API reference is
[`docs/agent/import.md`](../../agent/import.md).

## Starting point

- **The export was already reconcilable (DAT-02).** `ddcore export` writes NDJSON with child
  tables nested, an attachment manifest carrying each file's sha256, and a `manifest.json` of
  counts, checksums and versions. Nothing read it back.
- **`Insert` is built for new documents, and says so.** It stamps `owner`, `creation`,
  `modified` and `modified_by` with now and the current user (`doc.go:459`), regenerates a
  series id over the one the document has (`idgen.go:29`), runs every controller hook, and
  then queues webhooks, notifications and SSE. Docstatus 2 it refuses outright.
- **The only raw write was `PatchSQL`**, and only inside a migration patch.
- **Nothing was resumable.** `migrate`'s fixtures skip by id and re-run the whole insert path.
- **The 0.17 key rename had just landed**, so the export's key column is `id` and the
  reference columns are `*_id`. Exports taken before it say `name`.

## Key Decisions

### 1. A restricted write path in Go, never reachable from an app

`Ctx.ImportDoc` keeps `id`, `owner`, `creation`, `modified`, `modified_by`, `docstatus` 0/1/2,
`amended_from` and the workflow state, writes through the existing `writeInsert`/`columnValues`,
and runs no hook. It answers only to `Admin` or an unscoped System Manager, and is not exposed
through `host.go`.

**Rationale:** it can forge authorship and creation dates. That is exactly what loading history
requires and exactly what no app should ever be able to do. Keeping it in Go, behind an
administrative check, means the capability exists once, in one place.

### 2. Structural validation stays; hooks and effects go

`validate()` was split at the `validate` hook: everything after it is `validateFields`, which
the import runs with three switches — no `fetchFrom` (it would overwrite a historical value
with what the link says today), no mandatory check on `Password`/`Vault` (neither is ever
exported, so a required one could only fail), and a per-field link deferral.

**Rationale:** an import that skips casting and the unique keys has not loaded data, it has
filled a table. The hooks are the opposite case: they encode what should happen when something
happens *now*.

### 3. Effects are not suppressed one by one; the path simply never reaches them

No webhook, notification, Version, SSE or mail call exists on the import path. Two things that
would have leaked anyway are handled explicitly: `ddcore_series` is advanced past every id the
load writes, and a date notification whose day has already passed is marked done in
`ddcore_notification_due`.

**Rationale:** the sweep has no high-water mark by design (`notifications.go:400`), so
historical due dates would be found and delivered. Advancing the counter is the same class of
problem in reverse: the next document created on the site would collide with what was loaded.

### 4. Resumability comes from writing the cursor in the batch's own transaction

Each batch of `--batch` lines is one transaction; the cursor, the ledger rows, the series
advance and the due marks commit with it. A resume starts at the first line that did not
commit.

**Rationale:** any other arrangement makes the cursor a guess. A cursor written after the
commit can over-report after a crash; one written before can under-report. Writing it inside
is the only version that is exactly right.

### 5. One unloadable line costs itself, but the first attempt runs without savepoints

A batch that hits a write error is rolled back and replayed with a savepoint per record.

**Rationale:** a savepoint per record is the obvious design and the wrong default: past about
64 subtransactions, a transaction starts degrading snapshot handling for every other session in
the database. Most batches have no bad record, so most batches should pay nothing.

### 6. A dry run is one transaction for the whole load

Not one per batch, rolled back.

**Rationale:** found by running it. With a transaction per batch, a Project rolled back before
its Tasks were read made every link to it look broken, so `plan` reported errors a real run
would never hit. A rehearsal has to rehearse the real thing.

### 7. Idempotency is a ledger keyed by where the line came from

`ddcore_import_record` is keyed `(source_doctype, source_id)` with a unique index on the target
`(doctype, id)`. A document already on the site is compared field by field: identical is a skip,
different is a conflict.

**Rationale:** the ledger has to survive the mapping — the id on this site may not be the id in
the export. The `identical` default exists because `migrate` seeds roles and `Admin` before any
import runs, and a whole-site export carries those same rows; treating that as a conflict would
fail every real migration on its first line.

### 8. Links that no order can satisfy are verified once, at the end

A topological sort puts a link's target first. Self links, cycles and Dynamic Links are deferred
and checked afterwards with one anti-join per field.

**Rationale:** per-row checks during the load cannot see documents that are not in yet, and a
migration's graph always has cycles somewhere. One set-based query per field is also what makes
the check affordable on a real dataset.

### 9. Reconciliation compares decimals, not floats

Currency and Percent are summed per DocType and docstatus, as `big.Rat` built from the decimal
rendering of each rounded source value, against the database's own `sum()`.

**Rationale:** also found by running it. `SetFloat64(33.33)` carries the binary approximation,
and Postgres's numeric sum does not, so the two never agreed. The reconciliation exists for
money; it has to be exact about it.

### 10. Pre-0.17 exports are translated as they are read

When the manifest's version is below 0.17, `name` becomes `id` and `reference_name`,
`share_name`, `attached_to_name`, `target_name` and `docname` become their `_id` forms.

**Rationale:** those are precisely the archives a migration has on hand. Refusing them would
make the rename cost every site an extra export.

## Left Out

- **CSV and XLSX input, and a Desk screen.** The input is an export directory (DAT-01's
  demand-driven half).
- **Generated ids.** A record without an id is an error: identity is what is being preserved.
- **A `--hooks` mode.** Running an app's validate hooks over history would recompute values the
  export carries.
- **Importing `Audit Event`.** `Insert` refuses it and so does this path.
- **Password hashes.** They never leave a site, by DAT-02's contract.
- **Orphaned attachment bytes.** `Put` happens before the batch commits, so a crash can leave
  bytes whose row rolled back; a rerun writes the same key, and nothing sweeps orphans (the same
  residual as uploads, PRD-05).

## Verification Strategy

- Unit tests for the pure parts: the NDJSON reader (truncation, `json.Number`, the legacy
  translation), the mapping, the topological order, and `seriesCounterFor`.
- Engine tests against Postgres for `ImportDoc` (metadata, docstatus 2, no effects, deferred
  links, secrets, the series counter, the due marks) and for the run (idempotency, resume,
  per-record errors, dry run, the collision policies, mapping, exclusions, dangling links).
- Reconciliation tests for a clean load and for a changed amount, a deleted child, a missing
  row and a changed docstatus.
- An acceptance test that exports a seeded site whole, loads it into an empty one, reconciles,
  and loads it again.
- A manual round trip on the dev database — `demo` → `export --all` → `import plan|run|resume`
  → `reconcile` — which is what turned up decisions 6 and 9.
