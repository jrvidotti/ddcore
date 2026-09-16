# Delivered roadmap items: audit fixes

Spec: none. This plan comes from a read-only audit (2026-09-16) of the items merged
since 2026-09-13, comparing each `docs/agent/*.md` and `ROADMAP.md` row with the code.
The items are OPS-06 webhooks, OPS-03 notifications, OPS-05 assignments, PRD-06 audit,
OPS-01 print/PDF, OPS-04 workflows, the secret vault, and SEC-01 scopes. The goal is
that every delivered item's code matches its reference doc and roadmap row, and that
every remaining limitation is stated once, accurately. File:line references are from
commit `e148a4a` and may have drifted slightly, so re-locate them before editing.

## Global Constraints

- Repository rules (CLAUDE.md):
  - Server TypeScript is synchronous (never `await` in controllers or services).
  - Never hand-edit `.ddcore/types.d.ts`; regenerate with `./bin/ddcore types`.
  - Every user-facing string is an English key. After adding one, run
    `./bin/ddcore i18n extract --all --lang pt-BR` and fill in the pt-BR translation in the
    relevant `translations/pt-BR.csv`.
  - All comments and docs are in English.
- TDD: every code fix gets a regression test that fails before the fix. Go tests use the
  `ddcore_test` database on the `ddcore-pg` container (port 5455, `make docker-up` if down).
- Match surrounding code idiom and comment density. Docs follow the plain, unnumbered
  prose style of `docs/agent/mail.md`: no marketing words ("strictly", "graceful",
  "seamless").
- A doc states only what the code does. If a behavior is a limitation, say so plainly.
- Each task ends with one or more commits on branch `fix/delivered-items-audit`. Commit
  messages use conventional prefixes (`fix(scope):`, `docs:`), are in English, and end with
  `Co-Authored-By: Claude Opus 5 (1M context) <noreply@anthropic.com>`.
- Do not start a dev server. Run the Go package tests you touched plus
  `go build ./...`. Run `cd desk && npm run check` / desk vitest when you touch desk code.
- `ROADMAP.md` rows live in the "Available foundations" table. When a task edits a row,
  keep the two-column shape ("Delivered foundation" | "Remaining work").

---

### Task 1: Vault and integration secrets

Code fixes:
1. **Master key exposed to apps.** `ddcore.secret("key")` reads `DDCORE_SECRET_KEY`,
   because `Engine.Secret` (`internal/engine/secrets.go:50`) maps names through
   `SecretEnvName` with prefix `DDCORE_SECRET_`, and the vault master key is
   `MasterKeyEnvName = "DDCORE_SECRET_KEY"` (`internal/engine/vault.go:21`).
   - Make `Secret` (and `RequireSecret`) refuse any name whose env name equals
     `MasterKeyEnvName`, returning not-set.
   - Make `SecretNames` (`secrets.go:71`) exclude it, so `ddcore doctor`'s integration
     secrets list (`cmd/ddcore/doctor.go:~167`) no longer shows `KEY`.
   - Add a Go test.
2. **Renames orphan vault secrets.** Vault keys default to `DocType:field:name`, and
   `Rename` (`internal/engine/doc.go:~988-1060`) never touches `ddcore_vault`.
   - In the rename transaction, re-key rows whose key starts with `<DocType>:` and ends
     with `:<oldname>` (default key shape only) to the new name.
   - Look at how vault keys are built in `internal/engine/vault.go` before choosing the
     match; mirror the delete cleanup in `doc.go:~931-953`.
   - Add a Go test: a doc with a Vault field is renamed, and the secret is still readable
     under the new name.

Doc fixes:
- **`docs/agent/vault.md`:**
  - The key is any non-empty string, trimmed and hashed with SHA-256; hex is not decoded
    (`vault.go:23-31`). The doc wrongly asks for 32 bytes or 64 hex chars.
  - Audit coverage: `vault.list` is not audited (`vault.go:198-227`), and webhook signing
    reads skip the audit (`vaultRead`, `vault.go:~155`). A failure to write the audit
    entry is ignored (`vault.go:71`).
  - The audit link is `/app/Audit Event`, not `/app/audit-event`.
  - Limitations:
    - No key rotation; a different `DDCORE_SECRET_KEY` makes every secret fail to
      decrypt. Doctor warns only when the key is missing.
    - Backups are useless without the separately provisioned key.
    - `ddcore.vault.*` has no permission check; server code is trusted with any key.
    - Doctor prints vault key names.
    - A custom key template, or removing a child row during an update, still orphans
      secrets. Renames re-key default-shaped keys after fix 2.
  - The Vault field write formats accepted (`vault.go:321-354`): `{value}` /
    `{secret}` objects, `__vault_clear`, and an empty string, which keeps the old secret.
    Verify in code before writing.
  - `ddcore.secret` cannot read the master key (fix 1).
- **`docs/agent/fieldtypes.md` ~line 53:** remove the claim that access is audited in
  `tab_vault_audit_log`; it goes to `Audit Event`.
- **`docs/agent/ops.md`:** add `DDCORE_SECRET_KEY` and the vault doctor section, mirroring
  how other env vars and doctor sections are documented there. Verify doctor output in
  `cmd/ddcore/doctor.go`.
- **`ROADMAP.md`:** the SEC-06 row says "Add encryption/key rotation only for demonstrated
  storage requirements". Rewrite it:
  - The delivered foundation now includes encrypted `Vault` fields and `ddcore.vault.*`
    (link `docs/agent/vault.md`).
  - Remaining work: no key rotation; custom-template and child-row orphaning;
    `vault.list` unaudited; user-entered `Password` fields still plaintext.
- **Stale vault lines:** `docs/frappe-port-inventory.md:~106` ("No at-rest vault
  encryption") and `docs/frappe-implemented-features.md:~132-133` ("there is no encrypted
  vault") should reflect the vault.

### Task 2: Administrative audit (PRD-06)

Code fixes:
1. **`Audit Event` is mutable through `DBSet`.** `Insert`/`Save`/`Delete` refuse
   `Audit Event` (`internal/engine/doc.go:~401,~529,~899`), but `DBSet` (`doc.go:~808`)
   does not, so `ddcore.db.setValue` and `doc.dbSet` can alter entries.
   - Add the same refusal and message to `DBSet`.
   - Extend `internal/engine/audit_test.go`.
2. **`ddcore audit list --action` help text** says "action prefix or exact match", but the
   filter is exact (`internal/engine/audit.go:~123`). Fix the help text in
   `cmd/ddcore/audit.go`.

Doc fixes (`docs/agent/audit.md`), each verified against the call sites
(`grep -rn 'Audit(\|AuditDenied(\|RecordAudit(' internal core`):
- **Taxonomy table: missing and wrong rows.**
  - Add `workflow.transition`: target is the document; `Allowed` on success, `Denied` for
    wrong role / self-approval / failed condition; detail `{action, from_state, to_state}`
    (`internal/engine/workflow.go:~124,130,145,177`).
  - `permission.scope_grant` / `permission.scope_revoke` rows already exist; keep them.
  - `account.enable` / `account.disable` carry no detail (`core/doctypes/user/user.controller.ts:~38,40`).
  - `role.assign` also fires per role when a User is inserted.
  - `webhook.replay` detail is `{webhook, previous_status}` and has a `Denied` outcome for
    the role check (`internal/engine/webhooks.go:~500,530`).
  - `job.purge` detail includes `doneDays`, `failedDays` (`internal/engine/jobs_admin.go:~182-188`).
  - `method.<path>` Denied has empty target and is written when the method's `roles`
    option fails (`internal/api/api.go:~889-898`).
- **App-authored entries.** Document `ddcore.audit(...)` and `ddcore.auditDenied(...)`:
  signatures from `packages/sdk/src/index.ts:~112,116` and `internal/engine/host.go:~506-518`.
  State that apps can therefore write entries: the log is append-only, not engine-only.
- **Immutability.** Insert, Save, Delete and (after fix 1) DBSet are refused. Remove
  "Only internal engine operations write audit entries".
- **Transaction safety.** `job.cancel`/`job.retry`/`job.purge` are written directly on the
  pool with `RecordAudit` (`jobs_admin.go:~64,112,182`). Vault access without a request
  context is recorded on the pool as actor `System` (`vault.go:~68-79`).
- **CLI.**
  - Document `--target` (`cmd/ddcore/audit.go:~45`).
  - `audit purge` without `--days` falls back to `ops.auditRetentionDays` and errors when
    that is 0 (`audit.go:~136-146`).
  - The retention sweep `core.services.audit.sweep` only runs when the scheduler is
    enabled.

Other docs:
- **`docs/agent/ops.md`:** add `auditRetentionDays` to the `ops` JSON example and the
  retention section. The text saying "two retention windows" is stale: there are now
  job done/failed, webhooks and audit. Negative values are rejected at boot
  (`internal/config/ops.go:~113-123`); verify the exact set of keys there.
- **`ROADMAP.md` PRD-06 row:**
  - Remove the stale "Document approval … audit events arrive with … OPS-04".
    Approval transitions are now audited.
  - Add scope grants/revokes and workflow transitions to the covered list.
  - Remaining: role changes made via `dbSet`/`setValue` on User are not audited; job
    administration entries are written outside the caller transaction; no tamper-evident
    chaining or SIEM export; import and sharing audit wait on DAT-01/SEC-03.

### Task 3: SEC-01 scopes — Dynamic Link on direct access

Code fix: `checkUserPermissionsFor` (`internal/engine/perm.go:~208`) checks `Link` fields
and child rows, but not `Dynamic Link`. `scopeFilters` (same file, ~320) does filter them.
A probe confirmed the gap: a user scoped to company `Alfa` gets 0 rows listing
`Test Dynamic Record`, but `GetDoc` of a record with `party_type="Test Company"`,
`party_name="Beta"` succeeds.
- **Fix:** for each `Dynamic Link` field whose options field exists on the DocType and whose
  value in the document equals the `allow` DocType, require the Dynamic Link value to be
  in the allowed set. An empty value is out of scope, the same as for `Link`. Selector
  values naming other DocTypes are not restricted by that rule.
- **Test:** in `internal/engine/sec01_test.go` (reuse `setupSEC01` and its
  `Test Dynamic Record` fixture), assert that GetDoc of the out-of-scope dynamic record
  returns a PermissionError, that the in-scope one and a `User`-typed one still load, and
  that Update moving `party_name` to an out-of-scope company is refused.

Doc fixes (`docs/agent/scopes.md`):
- Remove the "Dynamic Link is checked in queries only" limitation.
- The "How a rule matches" Dynamic Link bullet now covers direct reads and writes too.
- In `ROADMAP.md` SEC-01 row, remove that residual.

### Task 4: Outgoing webhooks (OPS-06) and scopes

Code fix: webhooks bypass SEC-01. A System Manager with `User Permission` rows can still:
- create a `Webhook` on any DocType and receive every document at an external URL
  (`ValidateWebhook`, `internal/engine/webhooks.go:~537-580`);
- read out-of-scope payloads in `Webhook Delivery` (`reference_doctype`/`reference_name`
  are Data fields, so scope filters do not apply;
  `core/doctypes/webhook_delivery/webhook_delivery.doctype.ts`);
- replay them (`ReplayWebhookDelivery`, `webhooks.go:~499`).

Webhook administration becomes an unscoped-administrator function:
- A user whose `c.UserPermissions()` is non-empty is refused every permission type on
  `Webhook` and `Webhook Delivery`.
- Implement this centrally in `HasPermission` (`internal/engine/perm.go`) for those two
  DocTypes, so lists, reads, writes, export and replay all follow.
- Ensure `ReplayWebhookDelivery`'s refusal still writes the existing `AuditDenied`.
- Tests:
  - A scoped System Manager gets a PermissionError creating a Webhook, listing
    Webhook Delivery, and replaying.
  - An unscoped System Manager still can.
  - Use `internal/engine/webhooks_test.go` and/or `internal/api/sec01_test.go` following
    their existing setup.

Doc fixes (`docs/agent/webhooks.md`):
- Remove `Vault Audit Log` from the excluded DocTypes list (~line 28); the DocType no
  longer exists. Verify the list in `webhooks.go:~61-64`. Also fix the stale
  `Vault Audit Log` comment near `webhooks.go:~403`.
- Replay audit: only the role refusal writes a `Denied` entry. The other refusals (still
  `Queued`/`Retrying`, not found, webhook deleted) return errors without an audit entry.
  The detail is `{webhook, previous_status}`; actor and request id are columns.
- Scoped users cannot administer webhooks (fix above).
- Subscriptions are cached per process: a change saved from another process
  (`ddcore eval --commit`, `exec`, SQL) is not seen by a running server until restart
  (`webhooks.go:~109-166`).
- `Failed` also results, without retries, when the signing secret is missing or cannot be
  decrypted (for example after changing `DDCORE_SECRET_KEY`) (`webhooks.go:~365-369`).
- Saving a Webhook requires `DDCORE_SECRET_KEY`, because `secret` is a required Vault field.
- Document `ddcore webhooks list --limit` (default 20, `cmd/ddcore/webhooks.go:~43`), and
  the doctor section warning about failures in the last 24h.
- The payload redacts only Password/Vault fields and `*_password`/`*_secret` names; verify
  the exact rule in code.

Other docs:
- **`docs/agent/scopes.md`:** add Webhooks to the "Enforced surfaces" table (scoped users
  are refused webhook administration).
- **`ROADMAP.md` OPS-06 row:** remove the stale "`Audit Event` covers only webhook replay
  (extended … under PRD-06)". Add the per-process subscription cache and
  "administration refused to scoped users" to the remaining work and behavior.

### Task 5: Notifications (OPS-03) doc accuracy and exclusions

Code fix: `internal/js/notifications.go:~51` excludes the nonexistent DocTypes
`"Notification"` and `"Notification Occurrence"` as rule targets, while real internal
DocTypes are allowed.
- Replace them with the internal DocTypes that exist: `Webhook`, `Webhook Delivery`,
  `Audit Event`, `Email Delivery` (verify names in `core/doctypes`), keeping any already
  valid exclusions.
- Add or extend a test next to the existing notification definition-validation tests.

Doc fixes (`docs/agent/notifications.md`):
- ~Line 120-121 says there are no "assignments". OPS-05 delivers assignment notifications
  into the same inbox via `NotifyUser` (`internal/engine/notifications.go:~221-242`) and
  the core rule `core/notifications/todo_due.notification.ts`. Link `assignments`.
- Recipient access includes user access scopes: `notificationAccess` builds the user's
  own context, so scopes apply to recipient filtering, inbox, count, read state and the
  email recheck (`notifications.go:~60-100`). Link `scopes`.
- Single DocTypes are refused as targets (`internal/js/notifications.go:~57-59`).
- `condition` must return a boolean; a non-boolean throws and rolls back the save (verify
  in `internal/js/prelude.js` `evaluateNotification`).
- `email.args` is required whenever `email` is declared (verify in prelude).
- A recipient whose User has no email gets no email, silently (`notifications.go:~189`).
- No retention sweep for read notifications.
- The date sweep is queued every 5 minutes and only when the scheduler is enabled
  (`internal/engine/jobs.go`).

Other docs:
- **`docs/agent/scopes.md` Notifications row:** say that recipients and email rechecks
  also apply scopes, not only listing and counting.
- **`ROADMAP.md` OPS-03 row:** remove "assignments" from the missing list. Keep the other
  residuals.

### Task 6: Assignments (OPS-05)

Code fixes:
1. **"Assigned by me" and the document sidebar are broken for users other than
   System Manager.**
   - ToDo's `permissionQuery` returns `{allocated_to: user}`
     (`core/doctypes/todo/todo.controller.ts:~17-21`), and it is AND-ed into every
     `GetList`.
   - So `/api/todo/pending?scope=assigned_by_me` returns only self-assigned tasks, and
     `GET /api/assignments/{doctype}/{name}` shows only the caller's own allocations.
   - Fix in `internal/api/assignments.go`:
     - `listPending` and `forDoc` query with `IgnorePermissions: true` plus an explicit
       participant filter: `assigned_by = user` for assigned_by_me, `allocated_to = user`
       otherwise.
     - For `forDoc`, return every non-cancelled ToDo of a document the caller can read,
       since the caller already needs read access on the referenced document.
     - Keep the existing per-row read recheck of the referenced document.
     - Remember `IgnorePermissions` on ListArgs still applies scope filters (by design).
   - The generic `/api/resource/ToDo` listing stays allocated-to-only.
   - Tests in `internal/api/assignments_test.go`:
     - an assigner who is not a System Manager sees the task under `assigned_by_me`;
     - a second assignee sees other assignees in the document sidebar;
     - a user without read access on the document sees nothing.
2. **`assigned_by` spoofing.** `beforeInsert` fills `assigned_by` only when it is empty
   (`todo.controller.ts:~12-16`), so a user creating a ToDo via `/api/resource/ToDo` can
   name someone else as assigner. Always set `assigned_by = ddcore.user()` unless the user
   has System Manager. Add a test.
3. **Assignment notification text.** The text is built with `fmt.Sprintf`, so it is
   English-only (`internal/api/assignments.go:~88-91`), and the `NotifyUser` error is
   discarded (`_ =`).
   - Build subject and body as English keys translated for the recipient. Look at how
     `NotifyUser` / notification rendering handles language, and use the existing
     translation helper (`c.T` or the recipient-language equivalent).
   - Log the error with the engine logger instead of discarding it.
   - A failed timeline comment insert (`assignments.go:~84-86`) is also logged, not
     swallowed.
   - Run `./bin/ddcore i18n extract --all --lang pt-BR` and translate new keys.

Doc fixes (`docs/agent/assignments.md`):
- Revoke is allowed for the assigner, the assignee, or System Manager
  (`assignments.go:~170`).
- Pending `status` defaults to `Open`; pass `status=all` for everything
  (`assignments.go:~218-221`).
- Revoke returns `{success: true}`.
- No notification is sent for self-assignment or when the assignee cannot read the
  document.
- The due-date reminder sweep runs every 5 minutes when the scheduler is on, not "daily".
- Replace the `referenceFields … rename.go` cascade description: rename updates through
  `coreRefs` (`internal/engine/rename.go:~33`), and deleting a document deletes its ToDos
  with a direct delete, with no ToDo hooks.
- **Assign rules:**
  - read access on the target is required;
  - `allocated_to` must be an existing enabled user;
  - unknown JSON fields are rejected;
  - priority defaults to `Medium`.
- **`forDoc`:** excludes Cancelled tasks and returns at most 100.
- **Pending:** loads at most 1000 candidates before the access recheck and paging, so
  heavy users can see truncated results. `total` is the post-filter count.
- **Generic ToDo CRUD rules via `/api/resource/ToDo`:**
  - read/write for assigner or assignee;
  - delete for the assigner;
  - listing shows tasks allocated to the caller;
  - `assigned_by` is set to the creator (fix 2).
- `scope` values: `assigned_by_me`; anything else means assigned to me.
- Assignments are HTTP/Desk only: there is no server `ddcore.assign*`, no CLI or MCP
  tool.
- Assignment actions write no `Audit Event`, and a second open assignment to the same
  user is not prevented.

ROADMAP OPS-05 row: add the residuals above (the pending 1000-candidate cap, no audit
events, no duplicate prevention, HTTP/Desk only).

### Task 7: Approval workflows (OPS-04)

Code fixes:
1. **Insert bypass.** `Insert` (`internal/engine/doc.go:~419-436`) accepts `docstatus: 1`
   on a workflow DocType, so `POST /api/resource/X {docstatus:1}` or
   `ddcore.newDoc(...).submit()` creates a submitted document still in the initial state.
   - When a workflow applies and neither `opts.IgnorePermissions` nor
     `c.IgnorePermissions()` is set, refuse an insert whose docstatus differs from the
     initial state's docstatus.
   - Use a `cerr.Validation` English key in the style of the neighbouring
     "New {0} must start in initial workflow state '{1}'".
2. **Delete bypass.** `Delete` (`doc.go:~909`) only checks `delete` permission, so a
   document waiting for approval can be deleted by its owner.
   - When a workflow applies and permissions are not ignored, refuse delete unless the
     document's current state has docstatus 0 and the user's role may edit in that state
     (reuse the `allowEdit` check `HasPermission(...,"write")` applies in
     `internal/engine/perm.go:~159-171`).
   - Cancelled (docstatus 2) documents keep the existing rules.
3. **DBSet bypass.** `DBSet` (`doc.go:~808`) can change the workflow `stateField` or
   `docstatus`. Refuse writes to those columns on a workflow DocType unless
   `c.inWorkflowTransition` or `c.IgnorePermissions()`.
4. **Load-time validation.** Validate each workflow when definitions load (find where
   `defineWorkflow` registrations become `js.Workflow`, in `internal/js/prelude.js:~156-183`
   and the Go loader). Fail loading with a clear error when:
   - the DocType does not exist;
   - `stateField` is not a field of it;
   - a state has docstatus outside {0,1,2};
   - a state with docstatus 1 or 2 is used on a non-`submittable` DocType;
   - a transition goes from a docstatus-1 state to a docstatus-0 state, or leaves a
     docstatus-2 state (`Save` always rejects these; `doc.go:~540-552`).

   Check that `apps/testapp` workflows still load.
5. **i18n.**
   - `internal/i18nx/extract.go` `collectMeta` (~88-121) must collect workflow state names
     and action names as keys (the Desk renders `__(state)` and `__(action)`).
   - `desk/src/lib/components/FormView.svelte:~313` passes the raw action into
     `__("{0} {1}?", [action, name])`, and `desk/src/lib/form.svelte.ts:~348` the toast
     `Action '{0}' applied`. Translate the action before interpolating.
   - Translate the testapp pt-BR entries that extraction adds.
6. **Unsaved edits.** The workflow action buttons in `FormView.svelte` are only disabled
   while saving, and applying reloads the doc, dropping unsaved edits. Also disable them
   while the form is dirty; find the form's dirty flag in `form.svelte.ts`.
7. **Timeline comment.** The transition comment (`internal/engine/workflow.go:~180-190`)
   swallows insert errors and is built with `fmt.Sprintf`. Log the error, and build the
   text from an English key.

Tests: Go tests in `internal/engine/workflow_test.go` (or the existing workflow test
file) for 1-4. Desk vitest where a pure function is touched.

Doc fixes (`docs/agent/workflows.md`):
- Document the insert, delete and DBSet guards (1-3) and the load-time validation (4).
- A transition needs a role in `allowed` plus read access; it does not require the
  DocType's submit/cancel/write permission (`doc.go:~570`, `inWorkflowTransition`).
- Every transition runs the full save lifecycle (validate, beforeSave, onUpdate /
  onUpdateAfterSubmit, webhooks, notifications).
- The condition receives a plain JSON object, not a `Document`. A condition that throws
  hides the action from the available list; applying it returns the error without a
  Denied audit entry.
- `IgnorePermissions` skips the role check but not the self-approval check
  (`workflow.go:~109,129`).
- `docstatus` on a state is optional and defaults to 0.
- State and action names are translation keys, extracted by `i18n extract`.
- `GET /api/workflow/actions` on a DocType without a workflow returns
  `{state:"", allowEdit:true, actions:[]}`.
- The timeline comment names the user id.
- Remove "protects workflow integrity at the database layer" and similar overstatement;
  describe the actual guards.
- Mention `doc.applyWorkflow(action)` (added in `3e66ffa`) if not already there.

Other docs:
- **`docs/agent/controller-api.md`:** add `applyWorkflow(action)` to the Document methods
  list (~line 40); verify the signature in `packages/sdk/src`.
- **`docs/agent/conventions.md`:** add `workflows/<name>.workflow.ts` to the app layout
  (and `print/<name>.print.ts` if missing; Task 8 will not duplicate it).
- **`docs/agent/audit.md`:** `workflow.transition` should already be present from Task 2;
  verify the detail matches.
- **`ROADMAP.md` OPS-04 row:**
  - Replace "audit logging in timeline comments" with "`workflow.transition` audit events
    and timeline comments".
  - Remaining work: no pending-approvals inbox or approver notifications; linear
    state machines; no escalation.

### Task 8: Print templates and PDF (OPS-01)

Code fixes:
1. **`b.columns` breaks rendering.**
   - The JS builder emits `{type:"columns", columns:[[...]]}` (`internal/js/prelude.js:~433`),
     but `print.Block.Columns` is an `int` (`internal/print/blocks.go:~16`, used by
     `keyValues`), so unmarshal fails with "invalid print template output", and
     `RenderHTML` has no `columns` case.
   - Change the builder to emit `{type:"columns", cells:[[block,...],...]}` and add
     `Cells [][]Block \`json:"cells,omitempty"\`` to `Block`.
   - Render the cells as a CSS grid of equal columns, each cell rendering its blocks
     recursively.
   - Update the builder type in `packages/sdk/src/types.ts` if its shape is declared.
   - Test: render the columns example from `docs/agent/print.md` through the engine
     (`internal/engine/print_test.go`) and in `internal/print` unit tests.
2. **Hard-coded currency.** `formatPrintValue` (`internal/engine/print.go:~249-260`) prints
   `"R$ "` for any `pt*` language and `"USD "`-style otherwise. Use the site currency and
   the existing currency/number formatting helpers (DAT-06; look in `internal/num`,
   `internal/i18nx`, and how reports or the desk format Currency server-side). Test both
   an `en` and a `pt-BR` render with site currency USD.
3. **Page format and orientation.**
   - `page_format` (A4/Letter) and `landscape` (`internal/api/print.go:~107-113`) only reach
     Gotenberg, and the HTML hard-codes `@page { size: A4 portrait }`
     (`internal/print/render.go:~63-66`).
   - Emit `@page { size: <A4|Letter> <portrait|landscape> }` from the options, so Chrome
     and the command renderer honor them.
   - Test the HTML output.
4. **Letter Head default and selection.**
   - Only one default: add `core/doctypes/letter_head/letter_head.controller.ts` that, when
     a Letter Head is saved with `is_default`, clears `is_default` on the others (follow
     other core controllers' idiom; synchronous).
   - The default lookup (`internal/engine/print.go:~171`) gets deterministic ordering.
   - An explicit `letterhead=<name>` that is unknown or disabled returns a validation
     error instead of silently printing without one (`print.go:~176-178`).
   - `GET /api/letterheads` returns SQL errors instead of `[]` (`internal/api/print.go:~25-35`).
   - Tests.
5. **Routes without login.** `GET /api/print/formats/{doctype}` and `GET /api/letterheads`
   are registered outside `requireLogin` (`internal/api/api.go:~113-114`).
   - Require login for both.
   - `printFormats` also requires `read` permission on the DocType.
   - Tests in `internal/api`.
6. **i18n.**
   - `cerr.Unavailable` (`internal/cerr/cerr.go:~80`) is missing from `cerrConstructors`
     (`internal/i18nx/golang.go:~15-19`), so its messages are never extracted. Add it.
   - Print template `label`s are translated at runtime (`internal/engine/print.go:~42`) but
     not extracted. Collect them in `internal/i18nx/extract.go` `collectMeta`.
   - Run extraction and translate the new pt-BR keys (core and testapp).

Doc: rewrite `docs/agent/print.md` in the plain, unnumbered style of `mail.md`, stating
only what the code does:
- The builder vocabulary with exact signatures from `packages/sdk/src/types.ts:~172-189`:
  - `header(title, {subtitle, badge, badgeColor})` (there is no logo option);
  - `section(title, blocks)`, `keyValues(pairs, {columns: 2|3|4})`, `table(headers, rows, {aligns})`;
  - `totals(rows)`, `h(level, text)`, `rule()`, `pageBreak()`, `html` / `raw`, `columns` (fixed).
- **Standard layout:**
  - fields in a fixed 2-column grid, skipping Column/Tab Breaks and `Hidden` fields;
  - child tables use `inListView` fields, or the first fields up to 6;
  - a Draft/Submitted/Cancelled badge on submittable DocTypes only;
  - currency per fix 2.
- **Redaction:**
  - only Password and Vault fields are removed (`sanitizeDocForPrint`);
  - `Hidden` fields do reach custom templates;
  - field-level permissions wait on SEC-02;
  - read permission and user access scopes apply to the document.
- **Letter Head:**
  - fields including `align` and `image`;
  - one default (fix 4);
  - its HTML is inserted unescaped;
  - placed once before and after the content, not repeated per page.
- **PDF:**
  - renderer selection: Gotenberg via `DDCORE_GOTENBERG_URL`, `DDCORE_PDF_COMMAND`, or a
    discovered Chrome/Chromium/Brave/Edge, chosen once per process;
  - fixed concurrency: 5 for Gotenberg, 3 for Chrome/command;
  - Gotenberg 60s timeout;
  - 503 when nothing is installed or Gotenberg is unreachable; 500 for other renderer
    failures (Desk shows the browser-print hint only on 503).
- **Query parameters:** `letterhead`, `lang`, `page_format`, `landscape`, `download`
  (verify each in `internal/api/print.go`).
- **Template runtime:**
  - templates receive a real `Document` and full `ddcore.*`;
  - names are unique across apps;
  - `label` falls back to `name`.
- **Desk:** prints via the iframe's `contentWindow.print()`, and the language picker offers
  a fixed list.

Other docs:
- **`docs/agent/ops.md`:** add `DDCORE_GOTENBERG_URL` and `DDCORE_PDF_COMMAND`.
- **`docs/agent/conventions.md`:** add `print/<name>.print.ts` if Task 7 did not.
- **`docs/frappe-port-inventory.md:~140`:** stale "No print/template/PDF engine identified"
  for OPS-01.
- **`ROADMAP.md` OPS-01 row:** fix the delivered text (no "strict field-level redaction").
  Remaining work:
  - no running headers or page numbers;
  - Hidden fields reach custom templates until SEC-02;
  - no audit event for prints;
  - Letter Head HTML unescaped by design (System Manager content);
  - fixed renderer concurrency.

### Task 9: SEC-01 scopes — remaining bypasses found during execution

Added by the controller during execution. The Task 4 implementer found these gaps, and the controller confirmed them in code.

Code fixes:
1. **`Ctx.Exists` ignores scopes.** `Ctx.Exists(doctype, name)` (`internal/engine/query.go`, behind `ddcore.db.exists(doctype, name)`) runs a raw `SELECT 1 … WHERE name = $1`, so a scoped user can probe out-of-scope names. `docs/agent/scopes.md` lists `exists` as scope-enforced. `ExistsWhere` and `GetValues` already go through `GetList`, which applies `scopeFilters`.
   - Make the by-name form apply the same scope filters, for example by delegating to the `GetList` path with `IgnorePermissions: true` and a `name` filter.
   - Keep Task 4's webhook refusal behavior.
2. **`DBSet` ignores scopes.** `DBSet` (`internal/engine/doc.go`, behind `ddcore.db.setValue` and `doc.dbSet`) writes without any scope check.
   - When the context is not raised (`!c.IgnorePermissions()`) and the user has scope rows, refuse the write if the stored document is out of scope, or if the written values would move it out of scope. Use `checkUserPermissions` on the stored row, and on the stored row merged with the new values.
   - Before changing it, grep every framework-internal `DBSet`/`SetValue` caller (workflow transitions, notifications, assignments, mail/webhook status, rename, vault) and make sure each either runs in a raised context or legitimately acts for the user. A scoped user's normal operations must keep working.
3. **Scoped administrators can edit `User Permission`.** A System Manager with `User Permission` rows can create, change or delete `User Permission` documents, including their own, and so lift their own scope.
   - Refuse every permission type on `User Permission` to a user with scope rows, at both the role level and the scope level, following exactly the pattern Task 4 used for `Webhook`/`Webhook Delivery` (reuse its helper; do not duplicate it).
   - Unscoped System Managers and Administrator are unaffected.

Tests go in `internal/engine/sec01_test.go` (reuse `setupSEC01`), for a scoped user:
- `Exists` of an out-of-scope name is false and of an in-scope name is true;
- `DBSet` on an out-of-scope document is refused;
- `DBSet` moving an in-scope document's company to an out-of-scope value is refused;
- `DBSet` inside a document in scope succeeds;
- `Insert`/`Save`/`Delete` of `User Permission` is refused, including with `SaveOpts{IgnorePermissions:true}` and `GetList(... IgnorePermissions:true)`;
- an unscoped System Manager can still manage `User Permission`.

Doc fixes:
- `docs/agent/scopes.md`:
  - the app-code table covers `exists` (by name and by filters) and `db.setValue`/`dbSet`;
  - the enforced-surfaces table gets a `User Permission` administration row: scoped users are refused, so scope administration belongs to unscoped administrators;
  - the "Who is unrestricted" section is accurate.
- `ROADMAP.md` SEC-01 row: delivered text reflects the above.
