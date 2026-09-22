# Resumable Import and Reconciliation (DAT-01) Implementation Plan

## Context

ROADMAP Stage 2 lists DAT-01 as the prerequisite for a production migration: loading
legacy data has to be repeatable. DAT-02 already exports a reconcilable dataset (NDJSON +
`manifest.json` + `files/`), but nothing loads it back. The only write path, `Insert`,
rewrites `owner`/`creation`/`modified`, regenerates series ids, runs controller hooks and
fires webhooks, notifications and SSE. That is wrong for historical data. The goal is to
load an export directory into a site, optionally through a declarative mapping. The load
must keep identities and metadata, replay no external effects, resume after an
interruption, and be idempotent across runs. Afterwards it must reconcile counts, links,
statuses, files and monetary totals.

Built on top of the unreleased key rename (`7d24f51`, `name` → `id`): the document key is
`id`, id generation is `idGeneration` (`internal/engine/idgen.go`, `setID`, `nextInSeries`,
`formatID`, `nextCounter`), the reference columns are `reference_id`, `attached_to_id`,
`share_id`, `target_id` and `doc_id`, and `ddcore export` already writes an `id` column.
Everything below says `id`.

Decided with the user:
- **Source:** a DAT-02 export directory plus an optional JSON mapping. No CSV input and no Desk UI.
- **Per record:** structural validation only. No hooks, workflow guard, id generation, Versions, webhooks, notifications, mail or SSE.
- **Surfaces:** `ddcore import` CLI and an MCP tool. No HTTP endpoint.

## Workspace

Create a worktree based on local HEAD, following the memory note:
`git worktree add .worktrees/dat01-import -b feat/dat01-import HEAD`, then work inside it.
Commit per task, with a CHANGELOG `## Unreleased` entry in the same commit as the change.
Save this plan as `docs/superpowers/plans/2026-09-22-resumable-import.md`. Write the design
rationale to `docs/superpowers/specs/2026-09-22-resumable-import-design.md` (Starting point /
Key Decisions / Left Out / Verification Strategy, as in the SEC-03 spec).

## Design decisions

1. **Restricted write path, Go only.** Add `Ctx.ImportDoc(doc, ImportOpts)` in
   `internal/engine/import.go`. It is never exposed through `host.go` or the SDK, because it
   can forge `owner` and `creation`. Only `Admin`, or a System Manager with no User
   Permission scopes, may call it.
   - It keeps `id`, `owner`, `creation`, `modified`, `modified_by`, `docstatus` 0/1/2
     (1 and 2 only on submittable DocTypes), `amended_from` and the workflow state, and never
     calls `setID` (`idgen.go:29`).
   - It writes through `writeInsert`/`columnValues` (`doc.go:1838-1888`), which already keep
     the given metadata and round Currency.
   - Children go through a new `insertChildrenAsIs`: a plain INSERT, where missing child
     metadata defaults to the parent's and a child `id` that is already taken is a record
     error. Do **not** parameterise `writeChildren` (`doc.go:1952`), which renames taken rows
     and forces `modified`.
2. **Hook-free validation.** Split `validate()` (`doc.go:1375-1415`). Everything after the
   `validate` hook becomes `validateFields(d, doc, opts, fieldChecks)`, and `validate()`
   calls it, so behaviour does not change.
   - `fieldChecks` has three switches: `skipFetch` (no `fetchFrom`, so historical values are
     kept; this also covers `validateChildren:1736`), a per-target link filter
     (inline or deferred), and `skipSecretMandatory`.
   - Password and Vault fields are never exported, so a required one is reported as
     "secret not migrated" instead of failing.
3. **Links.** Sort DocTypes topologically over their Link and Table dependencies.
   - A Link to an earlier or unimported DocType is checked inline.
   - Links inside a cycle (self-links such as `amended_from`), Dynamic Links and the
     `coreRefs` soft references (`rename.go:17-43`) are deferred to a set-based anti-join
     pass at finalize.
   - Dangling links become errors on the run: its status is `completed_with_errors`, and
     reconcile exits non-zero.
4. **Mapping (`--map file.json`).** Per DocType it can:
   - rename it (`to`), rename or drop fields (`fields`), set constants (`set`) and remap
     values (`values`);
   - do the same for children;
   - remap ids (`ids`) and users (`users`), rewritten consistently across Link,
     Dynamic Link, the `coreRefs` pairs (`rename.go:17`), `owner`/`modified_by` and
     `attached_to_doctype`/`attached_to_id`;
   - set `onExisting: identical|skip|error`. The default is `identical`: when cast field
     values are equal, ignoring metadata, the record is skipped, otherwise it is a conflict.
     This keeps the Roles, Admin and Guest that `migrate` seeds from failing an `--all`
     import.

   An unknown source field is a record error unless the mapping drops it or sets
   `ignoreUnknown`.

   **Pre-0.17 exports.** When `manifest.ddcore` is below 0.17, the reader maps the key column
   `name` to `id` and the reference columns `reference_name`, `share_name`,
   `attached_to_name`, `target_name` and `docname` to their `_id` equivalents, then warns in
   `plan`. It is the same translation `migrate` does to a column, and without it every export
   taken before the rename is unreadable — exactly the archives a migration has on hand.
5. **Ledger and checkpoints.** Add three tables to `InternalSchema` (`internal/db/schema.go:14`):
   - `ddcore_import_run`: id, `manifest_sha`, `mapping_sha`, status, actor, started and
     finished, cursor jsonb (`{file: line}`), counts jsonb, `lease_until`;
   - `ddcore_import_record`: PK `(source_doctype, source_id)`, unique `(doctype, id)`,
     `run_id`, `line_sha`, status;
   - `ddcore_import_error`: `run_id`, doctype, line, id, message.

   `ddcore_import_record` joins the rename handling.

   Batches work like this:
   - Each batch of N lines runs in one transaction, and the cursor, the ledger, the series
     advance and the notification-due seeding commit inside it. Resume is therefore exact.
   - Batches run without savepoints. A write-time SQL error rolls the batch back and replays
     it in careful mode, with one `Ctx.WithSavepoint` per record (`engine.go:905`). That
     avoids the Postgres problems that set in past 64 subtransactions.
   - An advisory lock is held per run.
6. **Idempotency and effects.**
   - A rerun finds `(source_doctype, source_id)` in the ledger with the same `line_sha`
     and skips it.
   - A matching series or format id advances `ddcore_series` with
     `current = GREATEST(current, n)` inside the batch. This uses a new inverse of
     `nextInSeries`/`formatID`: `seriesCounterFor(d, doc)`, a regex built from the
     `idGeneration` series or format (`idgen.go:79-150`), honouring an `id_series` override.
   - Date notification rules are seeded into `ddcore_notification_due` per record when
     `due <= now`, using `notificationDue` (`notifications.go:363`). Future reminders stay live.
   - No webhooks, notifications, SSE, Versions or jobs are produced.
7. **Attachments.**
   - DAT-02 fix: the export copies bytes to `files/<storage key>` (`files/public/x`,
     `files/private/x`) instead of the basename (`cmd/ddcore/export.go:293`), and
     `manifest.json` gets `exportFormat: 2`.
   - The importer checks sha256 before calling `Store.Put` at `storage.KeyFromURL(fileUrl)`,
     so `file_url` is kept. For older exports it falls back to the basename, and a checksum
     mismatch there signals a collision.
   - File rows are enriched from `File.ndjson` when it is present, deduplicated against the
     documents' `_files`, and written in the owning record's unit.
   - A public URL with an extension in `dangerousExt` (`api.go:1342`) is a record error.
   - A dry run skips `Put`.
8. **Excluded by default** (the reason is shown in `plan`):
   - Audit Event: `Insert` refuses it.
   - Error Log, Webhook Delivery and Email Delivery: they record effects that already
     happened.
   - API Key: no secret is exported.
   - Webhook: its Vault secret is missing, and it would start firing.
   - Singles and top-level child files are refused.
   - Imported users have no password. `plan` and `reconcile` report how many, and users get
     access through invite/reset or SSO.
9. **Audit.**
   - `import.run` start and finish through `RecordAudit` with `cliActor()`.
   - The privilege changes the load makes are recorded with `detail.source="import"` and the
     run id: `role.assign` per Has Role row, `permission.scope_grant` and
     `permission.share_grant`.
   - Queue `invalidateUserPermissionCache` and `allSharesChanged`.
10. **Reconcile** is a read-only function. It compares the NDJSON with the database through
    the ledger:
    - rows and child rows per DocType;
    - file count, plus the stored-bytes sha256 with `--verify-bytes`;
    - dangling links and soft references;
    - the docstatus distribution;
    - each Currency/Percent field summed per docstatus with `big.Rat`, each source value
      first rounded by `num.Round` to the target precision.

    It writes a JSON report and exits non-zero on any mismatch.
11. **Maintenance and caches.** `--maintenance` follows the pattern at `backup.go:210-226`.
    The MCP tool sets `bypassMaintenance`, as migrate does (`migrate.go:66`). The docs say
    that a running server's in-memory caches need a restart or their TTL.

## Tasks (TDD: each test first)

1. **Export fix.** Change the attachment copy path and add `exportFormat: 2`. Extend
   `TestExportManifestsAttachments` (`internal/engine/export_test.go:479`) and add a CLI test
   with the same basename, one public and one private.
2. **Pure helpers** in `internal/engine/import_*.go`, with unit tests only:
   - an NDJSON reader over `bufio.Reader` with `UseNumber` that verifies the trailing
     `_manifest` line and each output sha against `manifest.json`, and translates a pre-0.17
     export's `name`/`*_name` columns;
   - mapping parse and apply;
   - the topological sort with cycle groups;
   - `seriesCounterFor`, covering dates, `{field}`, an `id_series` override, a counter in
     the middle of a format, and amended ids.
3. **Refactor** into `validate()` + `validateFields()` with no behaviour change. The whole
   `make test` suite is the check.
4. **Schema tables** and the rename hook for the ledger.
5. **`ImportDoc` and `insertChildrenAsIs`.** Build an engine fixture app modelled on
   `exportApp` (`export_test.go:25`) with:
   - Currency, a child table, a Link, a self-link, a Dynamic Link and an Attach field;
   - a submittable DocType with a series and a workflow;
   - a `uniqueKeys` key, a required Vault field and a date notification rule.

   Use a second database for export → import (extend `adminDSNFor`). Assert that metadata,
   docstatus 2 and the workflow state are preserved, and that no webhook, notification, job
   or SSE is produced.
6. **Batch runner** (`Engine.Import`). Tests:
   - a second run writes nothing and marks everything skipped;
   - resume after a forced stop at batch k gives identical counts, with no duplicate
     children or files;
   - a conflicting record logs an error and the rest of the batch loads;
   - the series counter moves past the imported ids;
   - a historical due date does not fire on `SweepNotifications`.
7. **Attachments.** Checksum, `Put`, File dedupe, the fallback for older exports and the
   dangerous-extension refusal.
8. **Deferred-link finalize pass** and the run status.
9. **Reconcile** (`Engine.ImportReconcile`). Tests: a clean load reconciles clean; a tampered
   amount, a deleted child and a missing file each produce a mismatch.
10. **CLI** in `cmd/ddcore/import.go`, with a `case "import"` and usage lines in `main.go`
    and flags through `newFlagSet`/`parseFlags` (`args.go`):
    `ddcore import plan|run|status|reconcile <dir> [--map] [--dry-run] [--batch 500]
    [--resume <run>] [--maintenance] [--only a,b] [--report f.json] [--verify-bytes]`.
    Add flag-parsing tests in `main_test.go`.
11. **MCP tool `import`** in `internal/mcp/mcp.go`, with `action`, `dir`, `map`, `dry_run`
    and `max_batches`, so a long load advances over repeated calls. Mention it in the
    server's Instructions.
12. **Acceptance test** in `internal/acceptance/import_test.go`: seed testapp (Projects with
    milestones, Tasks, Pedido with its series and workflow), export with `--all --children`,
    import into a fresh site twice, then reconcile.
13. **Docs and release notes:**
    - `docs/agent/import.md` (linked in `index.md` after `export`, and in the `cli.md`
      table), with a note in `export.md` on the new file layout;
    - the spec and plan files;
    - CHANGELOG `Added` (import) and `Changed` (the export's `files/` layout), alongside the
      key-rename entries already under `## Unreleased`;
    - ROADMAP lines 37, 51, 90 and 102, and `docs/frappe-port-inventory.md:117`;
    - `./bin/ddcore i18n extract` plus pt-BR translations for new user-facing messages.

## Residuals (record in ROADMAP)

- Generated ids from the mapping.
- A `--hooks` mode.
- Importing Audit Event.
- CSV/XLSX input and a UI.
- A detached MCP job.
- Password hashes are not migrated.
- Orphaned bytes from a crash between `Put` and commit (as in PRD-05).
- An unmeasured single-transaction dry run at millions of rows.
- Currency precision already lost to float64 on export.
- Stale caches in other processes.

## Verification

- `make test` (Go + TS), with the new engine, CLI and acceptance tests.
- `make check`, so no i18n keys are missing.
- A manual round trip in the worktree:
  1. `./bin/ddcore demo`
  2. `./bin/ddcore export --all --children --attachments --out /tmp/x`
  3. point a second `ddcore.json`/`DDCORE_DSN` at an empty database and run `migrate`
  4. `ddcore import plan /tmp/x`, then `ddcore import run /tmp/x --dry-run`
  5. `ddcore import run /tmp/x`; interrupt it with Ctrl-C and run it again with `--resume`
  6. run it a second time; it should report everything as skipped
  7. `ddcore import reconcile /tmp/x --verify-bytes` should exit 0
- Through MCP: the `import` tool with `action: "plan"` and `"status"`.
