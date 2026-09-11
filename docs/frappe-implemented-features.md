# Implemented Features in ddcore

Reference date: 2026-09-10. This document inventories only capabilities
with verified implementations in the current checkout. For gaps and evolution
priorities, see the [migration inventory](frappe-port-inventory.md).

## Overview

ddcore already executes TypeScript data apps on Postgres: DocTypes become
tables, forms, lists, and API; logic runs on the server; the Desk is generated from
metadata; jobs, reports, workspaces, translations, tests, CLI, and MCP are in the binary.

## Metadata, Database, and Documents

| Capability | Available Implementation |
|---|---|
| DocTypes in TypeScript | `defineDoctype` declares fields, naming, permissions, ordering, title, icon, and description; files are transpiled with esbuild and loaded into goja. |
| Postgres Schema | `ddcore migrate` creates/alters `tab_<snake_case>` tables, columns, and indexes; `--dry-run` reports the classified plan and `--prune` removes undeclared structures — only empty ones, refusing to drop anything that still has data. |
| App Migrations | Ordered two-phase patches (`beforeSchema`/`afterSchema`), with `ctx.sql` for batch backfills; fixtures, `afterInstall`, `afterMigrate`, and validation/ordering of `requires`. All within a single transaction. |
| Renames and Conversions | `renamedFrom` on a field and DocType renames column/table, indexes, and stored references; fieldtype changes that might lose data are refused until `convert` is declared. |
| Fieldtypes | Data, Email, Small Text, Text, Text Editor, Int, Float, Currency, Percent, Check, Date, Month, Datetime, Time, Select, Link, Dynamic Link, Table, Attach, JSON, Password, and layout fields. |
| Children and Naming | DocTypes `isChild`, `parent`/`parenttype`/`parentfield`/`idx`; series, field, hash, prompt, and format naming rules. |
| Server-Side Validation | Mandatory, uniqueness, Select, Email, Link/Dynamic Link, `fetchFrom`, dependencies, `mandatoryDependsOn`, and `allowOnSubmit`. |
| Document | Insert, save, submit, cancel, amend, rename, delete, reload, `append`, `dbSet`, field changes, and concurrency via `modified`. |
| History and Transactions | `docstatus`, `amended_from`, Version with diffs for `trackChanges`, Comment, and one transaction per request/job/test, with rollback on error. |
| Monetary Precision | Per-site precision (`currencyPrecision`, default = currency ISO minor unit) and rounding rule (`commercial`/`bankers`); `Currency` is rounded on write and the same rule applies in Go, app runtime, and desk. `Percent`, `Float`, and `Int` are excluded by contract. |
| Cross-App Extension | `extendDoctype` adds fields and overrides properties of a DocType from another app, with additive permissions and chained `hasPermission`/`permissionQuery`; conflicts between two apps reject loading. Form scripts of the extending app are appended to the owner's. |

References: [fieldtypes](agent/fieldtypes.md), [migrations](agent/migrations.md),
[controller API](agent/controller-api.md), [extensions](agent/extending.md),
[metadata](../internal/meta/meta.go), [merge](../internal/meta/extend.go),
[schema](../internal/db/schema.go), and
[Document](../internal/engine/doc.go).

## Business Logic, Permissions, and Identity

| Capability | Available Implementation |
|---|---|
| App Runtime | Controllers, services, reports, workspaces, patches, and tests in synchronous TypeScript on goja. |
| Hooks | `beforeValidate`, `validate`, `beforeSave`, `beforeInsert`, `afterInsert`, `onUpdate`, submit/cancel, `onUpdateAfterSubmit`, delete, and rename; also `docEvents` in manifest. |
| RPC | Controller methods and `whitelisted` functions, accessible via HTTP and Desk. |
| Bridge `ddcore.*` | Database, documents, messages/errors, session, roles, permissions, cache, synchronous HTTP, jobs, SSE, logs, and utilities. |
| Role-Based Permissions | `read`, `write`, `create`, `delete`, `submit`, `cancel`, `amend`, `report`, `export`, and `ifOwner`; Administrator bypasses checks. |
| App Rules | `hasPermission` and `permissionQuery` complement declarative rules. |
| Authentication | User, Role, Has Role, cookie session, CSRF, Argon2id password, and `key:secret` API key; inactive users/keys do not authenticate. |
| Login Protection | Attempts in `ddcore_login_attempt`, lockout by account and IP, 429 response with `Retry-After`, decoy hashing to prevent account enumeration, and `ddcore user unlock`. |
| Sessions and Keys | Session TTL and API key expiration from policy; `Secure` cookie decided per request; `ip`/`user_agent` recorded; revocation on password change, preserving self-service session. |
| Recovery and Invitations | 192-bit token stored in SHA-256, atomic single-use, four throttled public routes; generic response preventing address enumeration; invitation creates passwordless account. |
| Password Policy | Minimum length, maximum length, and refusal of password matching username, enforced at the hashing bottleneck — applies to form, CLI, recovery, invitation, and self-service. |
| Email | `internal/mail` with minimal SMTP, log transport, and dotted-path pluggable transport; delivery via `ddcore_job` queue. |
| Self-Service | Profile, language, password, active sessions, and personal API keys in `core/services/`, plus account administration in `users.ts`. |
| Integration Secrets | `ddcore.secret("name")` reads `DDCORE_SECRET_NAME` from environment; never stored in column, backup, export, or `Version`. |
| Cache | `ddcore.cache.get/set/del`, also used for roles, sessions, and API keys. |

References: [controller API](agent/controller-api.md), [access](agent/auth.md),
[permissions](../internal/engine/perm.go), [authentication](../internal/engine/auth.go),
[throttle](../internal/engine/throttle.go), [tokens](../internal/engine/token.go),
[email](../internal/mail/mail.go), and [bridge](../internal/engine/host.go).

## API, Files, and Realtime

| Capability | Available Implementation |
|---|---|
| REST | List, count, create, get, update, delete, submit, cancel, amend, rename, and execute methods. |
| Queries | Filters, `or_filters`, child table filters, selected fields, aggregations, grouping, ordering, pagination, and count. |
| Boot and Metadata | `/api/boot`, `/api/meta/:doctype`, and `/api/translations`, with boundary translation over HTTP, ETag, and language negotiation. |
| Link Search | Search by name, title, and configured fields, with filters and batch title resolution. |
| Reports and Dashboards | Endpoints for Script Reports, cards, and workspace charts, with applicable permissions. |
| Files | Multipart upload, File DocType, public/private paths, and authenticated private downloads. |
| Export | Streaming `GET /api/export/<DocType>` and `ddcore export` (also `--all`): streams full keyset-filtered dataset with child tables, attachment manifest with checksums, NDJSON or CSV, under `export` permission. |
| SSE | `/api/events` and `ddcore.publish` for document/list updates, progress, jobs, and user-targeted events. |
| Server and MCP | Embedded SPA and static assets; dev `/mcp` protected by administrative API key. |
| Probes and Correlation | Liveness (`/healthz`) separated from readiness (`/readyz`), which validates DB connection and returns 503; boolean body for anonymous requests and full report with queue, Error Log, and pool under System Manager. Sanitized `X-Request-Id` echoed in error envelopes, `request_id` column of Error Log, and `ddcore.session.requestId`. Access logging with duration and level filtering; panics convert to JSON error envelopes with captured stack trace. |

References: [CLI/HTTP API](agent/cli.md), [export](agent/export.md),
[operations](agent/ops.md), [API](../internal/api/api.go), and [event hub](../internal/engine/hub.go).

## Desk

| Capability | Available Implementation |
|---|---|
| Routes | Login, home page, workspace, list, new document, form, and report. |
| Lists | Metadata columns or `defineListView`, search, filters, ordering, pagination, count, Links, status indicators, and export of loaded page or full filtered dataset. |
| Forms | Layout by section/column/tab; fieldtype controls; computed states; save, submit, cancel, amend, delete, and rename. |
| Child Grid | Inline or dialog editing, add/remove, reordering, column widths, and properties customizable by form script. |
| Form Scripts | `defineForm`, `defineListView`, `frm.*`, Link filters, custom buttons, actions, indicators, async calls, and dynamic properties. |
| Interaction | Dialog, prompt, confirmation, messages, toasts, and standardized error display. |
| History | Sidebar with comments, versions, creation, and modification metadata. |
| Reports/Workspaces | Filters, table, totals, summary, bar/line/pie/donut charts, CSV, explicit sidebar, shortcuts, links, and cards. |
| Visual Localization | Currency, number, date, and status formatting; civil Date/Month and Datetime in site timezone. |

References: [form API](agent/form-api.md), [report API](agent/report-api.md),
[components](../desk/src/lib/components), and [controls](../desk/src/lib/controls).

## Jobs, Scheduler, and Internationalization

| Capability | Available Implementation |
|---|---|
| Persistent Queue | `ddcore.enqueue` creates jobs in Postgres; workers use `FOR UPDATE SKIP LOCKED`. |
| Scheduler | Cron and frequencies `all`, `hourly`, `daily`, `weekly`, and `monthly` declared in app manifest. |
| Job Robustness | Timeout, retries, result/error tracking, lease with heartbeat, and requeuing on worker interruption. |
| Job Management | `ddcore jobs list`, `jobs run <fn>`, and `jobs work`. |
| Queue Metrics | Counts by status distinguishing `runnable` from scheduled, age of oldest runnable job, expired leases (dead workers), and failure window, with thresholds in `ddcore.json` `ops` block. |
| Translations | English as canonical value; CSV catalogs in core/apps, extraction and validation via `ddcore i18n extract`. |
| Language and Timezone | Resolution via `X-Lang`, User, `Accept-Language`, and instance defaults; utilities and controls follow configured timezone. |
| Date Semantics | `Date`/`Month` are civil calendar dates that never shift; naive `Datetime` parsed in site timezone; `daily` triggers at site midnight; `Time` is validated. |

References: [jobs](../internal/engine/jobs.go), [i18n](agent/i18n.md), [CLI](agent/cli.md),
and [operations](agent/ops.md).

## Tooling and Example

| Capability | Available Implementation |
|---|---|
| Scaffold | `init`, `new-app`, and `scaffold_doctype`. |
| Development | `dev` with watcher/hot reload and `--auto-migrate`; `start` without watcher; configured via `ddcore.json`. |
| Types and Tests | `ddcore types` generates `.ddcore/types.d.ts`; `ddcore test` executes TS tests in rolled-back transactions; Go test suite covers engine, API, runtime, i18n, and HTTP acceptance. |
| Administration | `exec`, `eval` with default rollback, `user add`, `user passwd`, `apikey`, `demo`, and `doctor`. |
| MCP Stdio | Tools for meta, migration, data, methods, read-only SQL, evaluation, tests, logs, reload, and apps; resources for documentation and metadata. |
| Example App | [ddcore-demo](https://github.com/jrvidotti/ddcore-demo) contains Project, Task, Invoice and Project Settings, services, scheduler, scripts, workspace, report, translations, and tests. |

References: [CLI](agent/cli.md), [MCP](../internal/mcp/mcp.go),
[scaffold](../internal/scaffold/scaffold.go), [typegen](../internal/typegen/typegen.go), and
[ddcore-demo](https://github.com/jrvidotti/ddcore-demo).

## Surfaces Not Classified as Complete

- `isSingle` exists in DocType types, but the migrator does not create a Singleton table;
  this document does not consider persistent Settings implemented.
- `Password` is `text` at rest; values are not exposed via API reads, `Version`, or export,
  but there is no encrypted vault. Integration credentials belong in `.env` via `ddcore.secret`,
  not in a column.
- Email sending exists for password reset and invitations (minimal SMTP or app method), but
  it is not a full email product: no templating, delivery history, attachments, or outbox.
  Declarative notifications and configurable webhooks remain absent.
- Docstatus and controllers allow app-specific approvals; no declarative workflow engine is
  listed here.
- Fixtures still insert or skip by name: they do not update existing documents.
- Health probes, correlation, and `doctor --json` exist, but not a complete monitoring product:
  no Prometheus/OpenTelemetry endpoint, no per-route latency histograms, and no last-execution
  tracker for scheduler entries — `entries` only reports what the build would register, not
  that a live cron process is running. Deterministic ordering between DocTypes has been added.
- Large backfills hold migration locks for their entire duration; there is no out-of-transaction
  window, and the recommended path is queuing jobs and validating in the subsequent release.

## Evidence

This inventory was checked against `docs/agent/`, SDK, core, engine, API, Desk, and demo.
Code changes continue to require `make test` per [AGENTS.md](../AGENTS.md).
