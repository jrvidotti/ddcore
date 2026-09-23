# Post-v1 Inventory: ddcore Evolution and Frappe App Migration

Date: 2026-09-10. Inspected base: commit `ee437e9`, documentation and code in this
checkout. Source reference: Frappe v16.
Status: roadmap recommendation. The features proposed here were not implemented at the
date above; items marked **Implemented** were delivered later and link to their contract.

## 1. Recommended Direction

v1 fulfilled its defined goal: running an external app end-to-end with DocTypes,
business logic, Desk, reports, workspace, scheduler, and tests. The next milestone must be
**migrating a real operation with reconciled data, equivalent permissions, and rehearsed
rollback to the previous system**. App acceptance proves the core selected for
v1; coverage needed for other apps depends on their specific features and data.

I recommend this order:

1. Inventory the next Frappe app/site and choose a complete pilot workflow.
2. Prepare import, reconciliation, backup/restore, and access controls.
3. Complete Singleton configuration and secrets storage, when used.
4. Deliver printing/PDF, email/notifications, and workflows according to the chosen workflow.
5. Rehearse migration, perform user acceptance testing, and execute a controlled cutover.

The suggested scope for v2 is **migration and operation of administrative apps**, with
granular permissions and shared services that avoid duplicating infrastructure
in each app. Portals, visual builders, and comprehensive Frappe parity remain
contingent upon concrete demand.

### Scope and Limitations of the Analysis

- The state of ddcore was verified against canonical references in [agent/index.md](agent/index.md),
  the SDK, and the implementation. The MCP query `list_doctypes`, via the command in
  [.mcp.json](../.mcp.json), returned 11 DocTypes: eight in core and three in demo.
- The external repositories `alugueis` and `rent-frappe`, production data, and production
  integrations were not audited in this analysis. Priorities per app require
  that assessment; no credible timetable can be given before it.
- Official Frappe sources were consulted on the date above. They are living documentation:
  each source inventory must record the exact version and commit of the apps.
- "Existing" means an identified implementation exists, without asserting 100% compatibility
  with every Frappe nuance. "Partial" identifies a foundation that requires extension.
  "Not identified" means absent from inspected code/SDK, not proof of impossibility.

## 2. Review of the v1 Plan

Architecture decisions remain sound for the goal: Go as host, synchronous TypeScript
in goja, file-based metadata, Postgres, single tenant per instance, and metadata-driven Desk.
Subsequent features must preserve these contracts.

The plan explicitly deferred field-level permissions and User Permissions to v2.
It also did not include a data import product, configurable workflows, printing,
communication, or backup administration. These are post-v1 scope extensions.

There are differences between the historical document and the current checkout:

| In Plan / History | Current Evidence | Roadmap Consequence |
|---|---|---|
| Example app deferred | [ddcore-demo](https://github.com/jrvidotti/ddcore-demo) and [DEVELOPMENT.md](../DEVELOPMENT.md) | Use ddcore-demo as reference; validate domain logic in external apps. |
| `internal/doc`, `internal/perm`, `internal/jobs` | Responsibilities concentrated in [internal/engine](../internal/engine) | Plan modifications based on the actual structure. |
| Slimmer hooks and installation | `beforeInsert`, `onUpdateAfterSubmit`, fixtures, and `afterMigrate` in [SDK](../packages/sdk/src/types.ts) | Only map missing hooks or those with differing semantics. |
| Old review with defects | Regression tests in [fixes_test.go](../internal/engine/fixes_test.go), [perm_test.go](../internal/engine/perm_test.go), and [acceptance_test.go](../internal/acceptance/acceptance_test.go) | The old checklist (`check-v1-resolucao.md`) was removed from the repository; findings are tracked by the regression tests, without treating every historical finding as an active bug. |
| Timeout and retry planned | [jobs.go](../internal/engine/jobs.go) implements timeout, heartbeat, and lease recovery | Advance queue administration and observability; do not rewrite the worker. |

## 3. Existing Foundation

| Capability | Status and Evidence | Use in Migration |
|---|---|---|
| DocTypes, fields, links, and child tables | Existing: [meta](../internal/meta/meta.go), [schema](../internal/db/schema.go), [fieldtypes](agent/fieldtypes.md) | Rewrite source JSON/metadata into TS definitions and map types. |
| Document and lifecycle | Existing: [doc.go](../internal/engine/doc.go), [controller-api](agent/controller-api.md) | Preserve insert/save/submit/cancel/amend/rename rules and concurrency. |
| Business logic and services | Existing: [runtime](../internal/js/runtime.go), [bridge](../internal/engine/host.go) | Port Python to synchronous TS, verifying calls and hook side effects. |
| Basic permissions | Existing: [perm.go](../internal/engine/perm.go) | Reuse roles, `ifOwner`, `hasPermission`, and `permissionQuery`. |
| Login and authenticated integration | Existing: [auth.go](../internal/engine/auth.go) | Sessions, Argon2id, and API keys form the base; identity migration requires its own policy. |
| REST/RPC, uploads, and SSE | Existing: [api.go](../internal/api/api.go) and [hub.go](../internal/engine/hub.go) | Adapt consumers and preserve document/file privacy. |
| Desk, lists, forms, and scripts | Existing: [components](../desk/src/lib/components), [form-api](agent/form-api.md) | Recreate flows using metadata and Desk SDK. |
| Reports, cards, and workspaces | Existing: [report-api](agent/report-api.md) | Port queries and results; compare with identical reference dates. |
| Jobs and scheduler | Existing: [jobs.go](../internal/engine/jobs.go) | Port routines and test retry/interruption; external side effects require idempotency. |
| Schema, patches, fixtures, and dependencies | Existing: [migrate.go](../internal/engine/migrate.go), [fixes_test.go](../internal/engine/fixes_test.go) | Install and evolve apps; distinguish schema migration from legacy data loading. |
| History and comments | Existing: [core/doctypes](../core/doctypes) | Version/Comment aid operations; preserve source history as well. |
| Languages, dates, and developer tooling | Existing: [i18n](agent/i18n.md), [cli](agent/cli.md), [Makefile](../Makefile) | Retain canonical English, site timezone, type generation, MCP, and tests. |

## 4. Prioritized Feature Inventory

Priorities are proposed for the roadmap, not bug severities:

- **P0:** requirement for production cutover; can be met by documented operational
  tooling, without mandating a core UI or CLI command.
- **P1:** important capability to expand migration of administrative apps.
- **P2:** implement when use is demonstrated in the source app.

A P1/P2 item becomes a cutover blocker when it underpins a mandatory workflow,
an access control, or data that must be preserved.

### 4.1 Access, Identity, and Secrets

Frappe provides permissions by role, field level, and user Link values,
in addition to actions such as share, print, and import. ddcore already has the first
level; it needs to expand the model when these restrictions exist at the source.
Source: [Users and Permissions](https://docs.frappe.io/framework/user/en/basics/users-and-permissions).

| ID | Capability | Status in ddcore | Priority and Minimum Delivery |
|---|---|---|---|
| SEC-01 | User Permissions: restrict company, unit, portfolio, etc. per user | Partial: hooks allow specific logic; no equivalent generic entity/configuration. | P1; blocks apps with segregation. Configure scopes and enforce in read, search, write, reports, export, and events. |
| SEC-02 | Field-level permission (`permlevel` or equivalent) | Not identified in `FieldDef`/`PermDef`. `hidden` and `readOnly` do not replace read authorization. | P1; blocks restricted fields. Omit unauthorized fields from response and reject writes; include children, history, reports, and downloads. |
| SEC-03 | Document-level sharing and permissions administration | Not identified: no DocShare, role profiles, or generic permissions editor. | P1 for sharing; P2 for editors/profiles. Define precedence, revocation, and audit trail without conflicting metadata sources. |
| SEC-04 | Password reset, invitations, and login protection | **Implemented**: access policy declared in `auth` in `ddcore.json` and environment (public URL, proxy, email) in `.env`. Login rate-limited by `ddcore_login_attempt` — check **prior** to lookup and Argon2, key based on input string, decoy hash for non-existent address — and returns **429** with `Retry-After`. Single source for session TTL (SQL and cookie), `Secure` cookie decided per request, `ip`/`user_agent` in session and `DropSessions(user, exceptSid)`. Recovery and invitations via 192-bit token stored in SHA-256, atomic single use in `ddcore_auth_token`, across four throttled public routes; `forgot-password` returns identical responses for known, unknown, and disabled addresses. Password policy at hashing bottleneck, enforced across all five paths. `internal/mail` with minimal SMTP and dotted-path pluggable transport. Self-service in `core/services/` (profile, language, password, sessions, keys) and admin in `users.ts`. Desk: avatar menu, `/app/profile`, "forgot password" and reset page. Contract in [auth](agent/auth.md). | Delivered, with abuse tested (`TestSEC04_*`). Residual: CSRF remains header-presence check and lacks `__Host-` prefix; no verification of changed email (changing email is a *rename*, since email is `name`); lacks admin UI to unlock accounts and list sessions of other users — table stores the data. MFA/SSO follow in SEC-05. |
| SEC-05 | MFA and SSO/OIDC/LDAP | Not identified. | P2; mandatory before cutover when required by organization. Prioritize provider actually in use. |
| SEC-06 | Integration secrets, Password fields, and the encrypted `Vault` | **Implemented**: environment-backed secrets — `ddcore.secret("stripe_key")` reads `DDCORE_SECRET_STRIPE_KEY` from environment (`.env` in dev, platform in prod), and prefix is the boundary preventing apps from reading DSN or SMTP password via same path. `ddcore doctor` lists names and never values. `Password` fields never exposed via reads: `RedactPassword` scrubs value on all API responses, augmenting preexisting exclusion from `Version` and export. Per-record credentials also have an at-rest option: the `Vault` fieldtype and `ddcore.vault.*` store an AES-256-GCM-encrypted secret in `ddcore_vault`, outside the document table, audited on read/write. Contract in [vault](agent/vault.md), [fieldtypes](agent/fieldtypes.md#secrets-ddcoresecret-not-a-column) and [configuration](../DEVELOPMENT.md). | Residual: environment-backed secrets live outside DB by design — not in backup, replica, export, or diff; rotation is redeployment, not migration. A `Password` field typed by a *person* remains plaintext in column, protected only on egress. The vault has no key rotation, no permission check inside `ddcore.vault.*`, and still orphans a secret behind a custom key template or a removed child row; for Frappe secrets migration, requesting the source encryption key separately from the data dump remains valid. |

Local evidence: [SDK types](../packages/sdk/src/types.ts), [permissions](../internal/engine/perm.go),
[authentication](../internal/engine/auth.go), [fieldtypes](agent/fieldtypes.md).
For Frappe secrets migration, document the requirement of the source encryption key,
without including it in the standard data dump. Source: [Site configuration](https://docs.frappe.io/framework/user/en/basics/site_config).

### 4.2 Data, Modeling, and Compatibility

| ID | Capability | Status in ddcore | Priority and Minimum Delivery |
|---|---|---|---|
| DAT-01 | Import with mapping, validation, and resumption | **Implemented**: `ddcore import plan\|run\|status\|reconcile` and the `import` MCP tool load a DAT-02 export directory through `Ctx.ImportDoc`, a Go-only path that preserves id/owner/creation/modified/docstatus (2 included) and runs no hook and no effect; `ddcore_import_run`/`_record`/`_error` hold the cursor, the per-line ledger and the findings, so a rerun writes nothing and an interrupted run resumes exactly; deferred links are verified after the load, attachments against their sha256, and `reconcile` compares rows, children, docstatus, files and Currency totals as exact decimals. Contract in [import](agent/import.md); design in [specs/2026-09-22-resumable-import-design.md](superpowers/specs/2026-09-22-resumable-import-design.md). Data Import loads CSV/XLSX from the Desk as ordinary saves (the `import` permission, dry run, per-row errors); see [data import](agent/data-import.md). | Residual: Data Import has no child-table rows, `.xls` or background job; for the migration loader, ids are never generated; `Audit Event` is not importable; secrets and password hashes are not migrated; an attachment's bytes can outlive a rolled-back record. |
| DAT-02 | Complete and reconcilable export | **Implemented**: `Ctx.Export` traverses keyset-filtered dataset with child tables and attachment manifest, exposed via `GET /api/export/<DocType>` (streaming, capped) and `ddcore export` (NDJSON/CSV, attachment bytes, checksums). Contract in [export](agent/export.md). | Delivered. Server-side `export` permission checked; `Password` fields and credential columns never exported. The corresponding import is delivered (DAT-01). |
| DAT-03 | Single DocType / Settings | Partial/incomplete: `isSingle` in SDK and meta; schema skips table creation, without identified equivalent path in Document/bridge. | P1; advance if pilot uses Settings. Persistence, defaults, read/write, permissions, Desk, and singleton tests. Do not claim support based solely on flag. |
| DAT-04 | Schema and data evolution with renames/conversions | **Implemented**: `renamedFrom` renames column and table with indexes and stored references; patches have `beforeSchema`/`afterSchema` phases and `ctx.sql` for backfills; type conversions that might lose data rejected until `convert` declared; drops run after patches and `--prune` only drops empty structures. Contract in [migrations](agent/migrations.md). | Residual: fixtures still insert or skip by name without updating; large backfills hold single transaction locks — queue and validate in next release; `ddcore_job.args` and `tab_version.data` not scanned on DocType rename. |
| DAT-05 | Composite uniqueness and business keys | **Implemented**: `uniqueKeys: [{ name, fields }]` on DocType becomes a partial unique index per key; index named after key, so reordering or renaming component field does not rebuild it, and removing declaration drops index without `--prune`. A row with any component null or empty falls outside key, matching `unique` on single field. Pre-check identifies conflicting doc; `23505` read by constraint name and returns identical message — underpinning concurrent transactions. Contract in [fieldtypes](agent/fieldtypes.md) and [migrations](agent/migrations.md). | Out of scope: child tables, `extendDoctype`, and pre-rejection when database already contains duplicates — in that case `CREATE UNIQUE INDEX` fails with Postgres error and nothing is applied. Residual: removing `unique: true` from a *field* leaves index orphaned; scan covers only `uk_` namespace. |
| DAT-06 | Decimal precision and date semantics | **Implemented**: per-site precision and rounding rule, `Currency` rounded on write, identical rule across Go/goja/desk over single vector table, `roundCurrency`/`splitAmount`, and `Datetime`/scheduler in site timezone. Contract in [fieldtypes](agent/fieldtypes.md) and [controller API](agent/controller-api.md). | No decimal API: double is exact up to 2^53 and covers currency. Remaining: `SET TIME ZONE` on connection and report totals/CSV. |
| DAT-07 | Trees (`is_tree`/NestedSet) and Virtual DocTypes | Not identified as complete features. | P2; advance for indispensable hierarchies or external entities. Define queries, integrity, and permissions; self-referencing Link does not automatically support trees. |
| DAT-08 | Additional field types and properties | **Implemented**: `Text Editor` is rich text — HTML cleaned on write by an allowlist in `internal/richtext` (bluemonday), edited with Tiptap, printed as markup instead of escaped tags, and read back as plain text where a value predates the change. Added `Markdown Editor` (goldmark, rendered late), `Code`, `Attach Image` (checked at upload and on save), `Color`, `Duration` (seconds) and `Rating` (stars), each with a desk control, list/diff formatting and print blocks. Contract in [fieldtypes](agent/fieldtypes.md); design in [specs/2026-09-22-rich-text-and-field-controls-design.md](superpowers/specs/2026-09-22-rich-text-and-field-controls-design.md). | Out of scope: Table MultiSelect, Geolocation, Signature, Barcode, Autocomplete and Phone (input masks cover it). Residual: no tables in rich text and no external image URLs (SSRF through the PDF renderer); a private image does not render in a server-side PDF; `Code` has no syntax highlighting; mail still escapes everything, so there is no rich-text block in a message; changing a Rating's number of stars does not rescale stored values. |
| DAT-09 | Custom Fields, Property Setters, Client/Server Scripts | **Implemented**: `extendDoctype` adds fields and alters properties of a DocType from another app in `extensions/<snake>.extend.ts`, with per-field and DocType-level property setters, additional roles, `hasPermission`/`permissionQuery`, and form script that appends to owner's. Merge occurs before meta validation, so columns, types, API, list, and form see single DocType. Conflicts (field collision, two apps modifying same property, host missing from `requires`) reject load rather than depending on installation order. Contract in [extending](agent/extending.md). | Versioned path delivered — where Frappe Custom Field and Property Setter land. `fieldname`, `fieldtype`, Link/Table `options`, and DocType identity remain owner's. Visual editor of persisted customizations remains P2; inventorying and converting source customizations remains migration work, not core. |

Local evidence: [schema.go](../internal/db/schema.go), [migrate.go](../internal/engine/migrate.go),
[rename.go](../internal/engine/rename.go), [export.go](../internal/engine/export.go),
[extend.go](../internal/meta/extend.go),
[host.go](../internal/engine/host.go), [ListView](../desk/src/lib/components/ListView.svelte),
[ReportView](../desk/src/lib/components/ReportView.svelte), [Control](../desk/src/lib/controls/Control.svelte).
Frappe references: [Single DocType](https://docs.frappe.io/framework/user/en/basics/doctypes/single-doctype),
[bulk import](https://docs.frappe.io/framework/user/en/guides/data/import-large-csv-file),
and [database migrations](https://docs.frappe.io/framework/user/en/database-migrations).

### 4.3 Daily Operations, Automation, and Integrations

| ID | Capability | Status in ddcore | Priority and Minimum Delivery |
|---|---|---|---|
| OPS-01 | Print Format and PDF | Implemented: a standard layout per DocType, app templates in `print/<name>.print.ts`, the core `Letter Head` DocType, HTML and PDF endpoints (Gotenberg, a configured command or a local Chrome), and a Desk preview. See [print](agent/print.md). | No running headers, footers or page numbers; `Hidden` fields reach custom templates until field-level permissions (SEC-02); prints are not audited. |
| OPS-02 | Email delivery and communication history | No native SMTP/API sending identified; `email.go` validates address syntax. | P1; configurable transport, persistent queue, retry, templates, authorized attachments, and delivery/failure logs. IMAP/inbox remains P2. |
| OPS-03 | Notifications by event, date, and recipient | Implemented: `defineNotification` event/date rules, transactional occurrences, authorized per-user inbox, read/unread state, optional template email, database deduplication and recipient-only SSE invalidation. See [notifications](agent/notifications.md). | No visual editor, individual preferences, push, assignments or custom notification events. Access is revalidated when reading and before email attempts; already-sent email cannot be withdrawn. |
| OPS-04 | Approval workflow | Implemented: `defineWorkflow` state declarations, transitions, actions, docstatus binding (0/1/2), role-based `allowEdit`, atomic transition engine under row locking, audit history (`tab_audit_event` + `tab_comment`), HTTP API, and Desk UI action buttons and state pills. See [workflows](agent/workflows.md). | P1 delivered. States, actions, roles, conditions, history, and docstatus binding, validated on server. |
| OPS-05 | Assignment/ToDo, assignees, and reminders | Partial: Comment/Version exist; Task/assignee belong to demo, without generic assignment service. | P1 if team operates by pending tasks. Assign/revoke/complete, due date, and "my pending tasks", without implicitly granting access. |
| OPS-06 | Webhooks and reliable third-party delivery | Partial: synchronous HTTP + queue; no declarative webhook capability identified. | P1; blocks active integrations. Delivery after commit, signing, idempotency, timeout, retries, and logged/controlled redelivery. |
| OPS-07 | Heavy/prepared reports and scheduled dispatch | Partial: Script Reports, filters, CSV, cards, and charts. | P2; background processing in jobs, secured results, expiration, totals, and explicit limits based on real volume. |
| OPS-08 | Global search and specialized views | Implemented (OPS-08): permission-aware global search (Mod+K palette, `globalSearch` opt-in/out, query-time with no index) and Calendar, Kanban, Gantt and Card list views through `defineListView`. No saved per-user view preferences on the server. | P2 by workflow; indexing with authorization and saved preferences before multiplying views. |
| OPS-09 | Auto Repeat and generic assignment rules | Partial: scheduler and services allow implementation in app. | P2; extract into shared service when two apps require identical semantics. |
| OPS-10 | Portal, Web Forms, and customer/supplier access | Partial: signed-in portals are delivered (`definePortal`, Website Users confined to `/portal`, pages as their only grant, protected attachments, per-user write limits; see `docs/agent/portal.md`). No anonymous Web Forms or self-registration. | P2 for public forms only; build them on the portal pages with Guest throttling and a review queue. |
| OPS-11 | API and library compatibility | Partial: similar REST/RPC model; server uses Go/goja and proprietary bridge. Differences against Frappe v15 (routes, payloads, envelopes, pagination, auth, errors, runtime dependencies) are mapped in the [REST gap analysis](frappe-rest-gap-analysis.md), which also holds the consumer inventory; no consumer audited yet. | P0 for active consumers. Map paths, verbs, payloads, errors, auth, and pagination; port Python/Jinja/SQL and replace incompatible Python/Node dependencies. |

Official sources for reference behavior: [printing and PDF](https://docs.frappe.io/framework/user/en/desk/printing),
[notifications](https://docs.frappe.io/framework/notifications),
[workflows](https://docs.frappe.io/erpnext/workflows),
[assignments and ToDos](https://docs.frappe.io/framework/assignments-and-todos),
and [webhooks](https://docs.frappe.io/framework/user/en/guides/integration/webhooks).
Workflow documentation is in the ERPNext manual; the requirement here is the shared
approval mechanism. Accounting, tax, inventory, and rental rules belong
in domain apps, not core.

### 4.4 Production, Recovery, and Evolution

| ID | Capability | Status in ddcore | Priority and Minimum Delivery |
|---|---|---|---|
| PRD-01 | Instance backup and restore | No dedicated CLI commands identified. | P0: automated procedure for database, public/private files, configuration, secrets, and versions; rehearsed restore. CLI command can follow later. |
| PRD-02 | Deployment, maintenance, and rollback | Partial: `start`, migrations, and per-instance configuration exist. | P0: versioned artifacts, TLS/proxy, process supervision, pause writes/jobs, and DB-compatible rollback procedure. |
| PRD-03 | Observability and operational health | **Implemented**: liveness (`/healthz`, `/api/health`) separated from readiness (`/readyz`, `/api/ready`), the latter proving DB with deadline and returning **503**; boolean body for anonymous requests and full report with queue, Error Log, and pool at `/api/health/report` under System Manager. Probes bypass expired API keys via exact path match. Every request receives `X-Request-Id` — sanitized at entry, echoed back, mirrored as `requestId` in error envelope, stored in `request_id` column of `Error Log`, and accessible to apps in `ddcore.session.requestId`. Access log with duration, level filtering, and `DDCORE_LOG_FORMAT=json`. Panics produce JSON envelope and `Error Log` entry with stack trace, replacing raw chi 500. Queue metrics distinguish `runnable` from scheduled and count expired leases. Thresholds in `ops` block of `ddcore.json`. `ddcore doctor [--json] [--strict]` probes DB before engine, reports instead of crashing when down, redacts DSN, and exits non-zero on critical issues. Contract in [operations](agent/ops.md). | Delivered. Residual: backup alerts depend on PRD-01/02 (nothing emits backup events yet); no Prometheus or OpenTelemetry `/metrics`; nothing persists last scheduler execution, so `entries` reflects what this build would install rather than an active cron. Job administration, and `ddcore_job.request_id`, arrived with PRD-04. |
| PRD-04 | Job administration | **Implemented**: `ddcore jobs list|show|stats|retry|cancel|purge|scheduled`; a System Manager-only HTTP surface (`GET /api/jobs`, `/api/jobs/{id}`, `/api/jobs/stats`; `POST /api/jobs/{id}/retry`, `/cancel`, `/api/jobs/purge`) and the MCP tools `list_jobs`, `get_job`, `retry_job`, `cancel_job`, `purge_jobs`. Cooperative cancellation of a running job interrupts the VM by the path the timeout already uses, rolling back its transaction; a cancelled job is never retried and writes no Error Log row. Retry inserts a new job linked by `retry_of`/`retried_as`, keeping the failure as the record and `job:<id>` naming one execution. Retention windows in the `ops` block (zero keeps forever), swept daily by `core.services.jobs.sweep` and on demand with `--dry-run`. `ddcore_job` gains `request_id`, `cancel_requested`, `cancelled_by`, `retry_of`, `retried_as`; `enqueue` accepts `maxAttempts`; `QueueHealth` counts cancellations apart from failures. Fixed on the way: the requeue branch stamped `finished` on queued rows, terminal writes were unfenced and ran on the worker's cancelled context, `requeueStale` clobbered `error`, and shutdown was recorded as job failure. Contract in [operations](agent/ops.md). | Delivered. Residual: no Desk screen; no job priority or per-queue worker affinity; a job blocked inside a host call is not interruptible, so cancellation latency is unbounded for it; payloads (`args`, `result`) are readable only through `ddcore jobs show`; nothing is exactly-once — a cancel rolls back database work but no external effect, and a retry may repeat one. |
| PRD-05 | File lifecycle | Partial: File DocType, uploads, public/private paths, local or S3-compatible storage, byte deletion with the File. | P0: copy bytes, validate checksums, preserve links and access permissions. P2: quotas and retention policies on demand. |
| PRD-06 | Auditing beyond Version | Partial: per-document diffs and comments. | P1; blocks those needing specific audit trails. Record permission changes, imports, approvals, and administrative actions, protecting sensitive data. |
| PRD-07 | Core/app compatibility contract | Partial: `requires`, manifest versions, generated SDK, and tests. | P1: supported version ranges, changelog, update verification, and consumer tests; pin versions in pilot from P0. |
| PRD-08 | Multisite and multiple replicas | Single tenant per instance is a v1 decision; local events/cache are part of architecture. | P2. First automate isolated instances. Before multiple replicas of same tenant, validate scheduler, cache, events, and job distribution. |

Local evidence: [CLI](../cmd/ddcore/main.go), [doctor](../cmd/ddcore/doctor.go),
[configuration](../internal/config/config.go), [API](../internal/api/api.go),
[probes](../internal/api/health.go), [correlation](../internal/api/observe.go),
[health](../internal/engine/health.go), [DB probe](../internal/db/health.go),
[jobs](../internal/engine/jobs.go), and [hub](../internal/engine/hub.go).
Frappe operational reference: [Bench commands](https://docs.frappe.io/framework/user/en/bench/resources/bench-commands-cheatsheet).

## 5. Acceptance Criteria for Priority Components

The details below are proposed requirements for ddcore; they do not represent guarantees
that Frappe or v1 already satisfies them across all scenarios.

| Delivery | Minimum Evidence for Acceptance |
|---|---|
| Granular Permissions | Two users with the same role and different scopes cannot access each other's documents/fields via URL, REST, search, report, export, history, files, or SSE. Also test administrative contexts and explicitly privileged jobs. |
| Importer | Two runs of the same batch preserve counts, names, and totals; interruption and resumption do not duplicate children or external side effects; errors identify source, field, and reason. |
| Single / Settings | One configuration per DocType/instance; save, reload, query via API, apply defaults, and deny unauthorized access function without missing document tables. |
| Secrets | Standard read/API/export/Version/log never reveals value; retrieving and rotating secret continues functioning after restore. |
| Printing | Long document with child rows, accented characters, formatted values, timezone, and multiple pages produces readable output; download denied for unauthorized user and omits restricted fields. |
| Email / Notifications / Webhooks | Rollback suppresses sending; worker crash allows resumption; retries use stable key and are audited. SMTP does not promise exactly-once delivery: document duplicate potential on ambiguous outcomes. |
| Workflow | Directly altering state field or invoking submit via API does not bypass approval; two simultaneous approvals do not trigger effects twice. |
| Backup / Rollback | Restoring to isolated instance recovers database, attachments, and configuration; login and critical workflows succeed within defined recovery time objectives. |

## 6. Migration Guide for an App/Site

### Stage A — Discover Source and Lock Contract

Choose an app and a workflow with clear start, completion, and verifiable operational effect.
For `alugueis`, candidate to confirm in external repo: registration → contract →
billing → payment settlement → receipt → report. Use an isolated copy of source data.

Complete a manifest per site containing:

- versions/commits of Frappe and each app, database, volume, and rate of change;
- standard/custom DocTypes, singles, trees, child tables, record counts, and attachment size;
- effective metadata, including Custom Fields, Property Setters, permissions, and defaults;
- controllers, hooks, scripts, patches, fixtures, reports, print formats, and SQL;
- users/roles/scopes, shares, workflows, assignments, and private documents;
- scheduler, pending jobs, inbound/outbound integrations, and API consumers;
- language, timezone, currency precision, Select values, and history policies;
- owner for acceptance of each workflow, acceptable downtime, recovery, and rollback procedures.

For each dependency, record: Frappe feature → usage evidence → ID in this
inventory → ddcore solution → equivalence test → owner → blocks cutover?
A similar API or matching fieldtype name is not sufficient to claim equivalence.

### Stage B — Port Behavior and Prepare Target

Port app rules and tests to synchronous TS; port form scripts to the Desk SDK.
Identify differences in hook execution order, `dbSet`, permissions, transactions,
SQL queries, and libraries. Adapt integrations via contract or a thin compatibility
layer when external consumers cannot be updated simultaneously.

Provision an isolated instance with pinned versions of core and app. Use
`migrate`/MCP for schema and supported tools for data. `ddcore migrate`
updates the target schema; it does not automatically convert a Frappe database.

### Stage C — Build Data Load and Reconcile

1. Extract data with deterministic pagination and a consistent snapshot or maintenance
   freeze window. Save manifest and checksums of extracted files.
2. Define a mapping table of DocType, field, name, type, and value. Preserve
   `name` when possible; if changed, remap all Link/Dynamic Link and references.
3. Load identities and configurations; then reference tables and documents in
   dependency order, with their child tables (`parent`, `parenttype`, `parentfield`, `idx`).
   Cyclic dependencies require phased loading and explicit final validation.
4. Preserve semantics of `owner`, `creation`, `modified`, `modified_by`, `docstatus`,
   and `amended_from`. If standard CRUD overwrites them, build a restricted migration
   mechanism with validation and auditing, rather than assuming `insert_doc` preserves them.
5. Handle submitted/cancelled history without re-triggering billing, journal entries, or dispatches.
   Choose per DocType between controlled replay and historical state restoration;
   in both, verify integrity and record which side effects were suppressed.
6. Migrate attachments and their permissions; compare checksums, links, and byte counts.
   Import or archive with defined access policies Version, Comment, Communication, and other
   audit trails that must remain queryable, preserving event provenance.
7. Map localized Selects to canonical English and generate catalogs. Convert civil
   dates and timestamps with explicit rules; do not shift Date by timezone.
8. Define initial user access. The current authenticator accepts its Argon2id format;
   do not assume Frappe hashes are compatible. Use password reset/invitation or
   tested temporary compatibility; reissue API keys and do not import active sessions.
9. Reconcile records, children, orphaned links, attachments, statuses, totals, and reports across
   historical reference dates. For monetary values, define rounding and tolerance per
   metric; do not rely merely on visual inspection or total row counts.

Exit criterion: all discrepancies explained and approved, zero broken references,
and no duplicates upon re-execution. Retain source, transform, and destination logs
for each batch to facilitate auditing.

### Stage D — Rehearse Operation and Rollback

Execute the complete sequence at least twice in an isolated environment: an initial
load and a repeat with interruption/resumption. Measure time, resources, and downtime
window under representative data volume. Perform UAT across user roles.

Compare Frappe and ddcore results using identical reference timestamps. During the
rehearsal, prevent target from dispatching live invoices, emails, or real webhooks. Comparison
may run in parallel, but there must be a single authoritative system for operational
writes; bi-directional synchronization would require an additional project.

Restore the backup on a third instance and execute the critical workflow. Document
the rollback decision checkpoint, owners, and handling of writes occurring
after cutover: the prior backup alone will not contain these newly recorded operations.

### Stage E — Cut Over and Monitor

Freeze source writes, pause scheduler/external consumers, capture final data batch
or delta (including deletions/cancellations), and reconcile again. Enable access
and integrations on target only after agreed criteria are met.

Keep previous Frappe instance in read-only access for the agreed retention period. Monitor
errors, queue, notifications, latency, and daily reconciliation. Decommission source
only after operational acceptance and confirmation of target recovery capabilities.

## 7. Suggested Milestone Sequence

| Milestone | Scope | Dependencies and Exit Condition |
|---|---|---|
| M0 — Pilot Assessment | Source manifest, contract differences, and applicable priorities | All critical workflows have owners and equivalence criteria; versions pinned. |
| M1 — Migration and Recovery | DAT-01/02/04/06/09, OPS-11, PRD-01/02/03/05, and identity requirements | Repeatable/reconciled load, restored backup, basic operations, and initial access demonstrated. |
| M2 — Pilot Capabilities | SEC-01/02/03/06, DAT-03/05, and OPS-01…06 as required | Zero required permissions, approvals, generated documents, or integrations without equivalents. May progress in parallel with M1 after M0. |
| M3 — Pilot in Production | Final rehearsal, user training, cutover, and monitoring | Workflows accepted, operational SLAs met, and rollback documented/rehearsed. |
| M4 — Broaden Migration | Second app with different requirements, PRD-07, and proven P2 items | Shared APIs reused, updates tested, and zero domain logic leaking into core. |

Do not tie cutover to completing all P1/P2 items. The gate is the set of
mandatory requirements for the chosen app. Likewise, do not omit an
indispensable feature merely because its general roadmap priority is P2.

## 8. What to Preserve as Product Decisions

- Single tenant per instance; provisioning automation before multisite in core.
- Postgres as a mandatory external service; queue, outbox, and notifications can leverage
  existing infrastructure. Email providers are configurable integrations.
- Structural metadata in files. Configuration and operational state can be stored
  as data; visual structure editors would require explicit export/versioning workflows.
- Synchronous TypeScript on the server and zero runtime Node/Python dependencies for apps
  in production. Library portability must be evaluated, not assumed.
- Printing requires a dedicated decision: an external HTML→PDF renderer introduces deployment
  dependencies. Evaluate Go libraries, optional headless browsers, or external services, demonstrating
  fidelity and resource overhead before promising both complex PDF rendering and self-contained binaries.
- Internationalization and server-enforced business logic remain core contracts.
  Migration must not reintroduce translated strings into stored data or rely on client-only validation.

## 9. Validation of this Inventory

Documentary verification: local links, consistency between current state and proposal, code
evidence, and official sources. MCP inspection of metadata only; no product data
migration was executed to generate this inventory.

Repository validation must include `make test` per [AGENTS.md](../AGENTS.md).
`make check` complements the gate with demo TypeScript and translation catalogs;
it is currently not a dependency of `make test` in the Makefile. Executing these commands
validates the checkout, but does not prove the future features listed here.

Results obtained during this analysis:

| Verification | Result |
|---|---|
| Local links across both docs and `git diff --check` | No broken links or whitespace issues. |
| `make test` | Failed: build, vet, and Go stage completed; demo with 25 tests, 2 failures. Target stops before Desk tests. |
| `make check` | Passed: Desk/demo types and catalogs with zero missing translations; pre-existing `autofocus` warning on login. |
| `make test-desk`, executed separately | Passed: 79 tests across 10 files. |

The two demo test failures were discrepancies between the test string assertion and
the returned error: [project.test.ts](https://github.com/jrvidotti/ddcore-demo/blob/main/doctypes/project/project.test.ts)
expected `final`, while the controller returns `The end date cannot be earlier than
the start date.`; [task.test.ts](https://github.com/jrvidotti/ddcore-demo/blob/main/doctypes/task/task.test.ts) expected
`limite`, while receiving `The due date cannot be earlier than the project start (…)`.
Validations correctly rejected invalid dates; string matching failed.
These were pre-existing test files, with no modifications in this documentation release. Aligning
tests with the error/language contract yields green `make test` prior to pilot.

Resolved subsequently: both expectations now match the English error strings emitted
by controllers, and `make test` runs fully green. The table above remains as a historical
record of what was measured on 2026-09-10, not the present state.
