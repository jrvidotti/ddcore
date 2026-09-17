# ddcore Feature Roadmap

Updated: September 17, 2026.

This roadmap prioritizes reusable framework capabilities by expected benefit relative
to implementation effort and ongoing maintenance. It is derived from the
[Frappe migration inventory](docs/frappe-port-inventory.md), with implementation
checks against this checkout. Inventory IDs provide traceability; they are not bug
severities or release commitments.

## Prioritization policy

- Favor features that complete an existing abstraction, remove repeated app code,
  or improve operation across many apps. Extend existing services before adding infrastructure.
- Within a stage, take the items in the listed order by default. Dependencies and
  demonstrated app requirements can move an item earlier; stages are not
  all-or-nothing release gates.
- Effort is relative: **S** is a focused extension, **M** crosses a few framework
  layers, and **L** introduces substantial cross-cutting behavior. These are planning
  estimates, not durations; validate them when designing each feature.
- Security, integrity, recovery, and mandatory operational flows override the default
  order before production. A demand-driven feature becomes a release blocker when
  the target app depends on it.
- No external app or production dataset has been audited for this roadmap. Avoid
  calendar promises and full Frappe compatibility claims.

## Available foundations

Build on DocTypes, lifecycle hooks, role permissions, REST/RPC, generated Desk,
reports, jobs/scheduler, files, Version/Comment, and the existing CLI/MCP tools.

| Inventory ID | Delivered foundation | Remaining work, separate from the delivered capability |
| --- | --- | --- |
| SEC-04 | Password recovery, invitations, login throttling, password policy, and account self-service. See [authentication](docs/agent/auth.md). | Strengthen CSRF validation; evaluate `__Host-` cookies with deployment constraints; add administrator account unlock/session controls and email-change verification. MFA/SSO remains SEC-05. |
| SEC-06 | Environment-backed integration secrets, Password redaction from ordinary API responses, export, and history, and encrypted per-record `Vault` fields with `ddcore.vault.*`. See [vault](docs/agent/vault.md) and [field types](docs/agent/fieldtypes.md). | No key rotation: a changed `DDCORE_SECRET_KEY` fails every existing secret to decrypt. A custom vault key template, and removing a child row during an update, still orphan a secret. `vault.list` is not audited. User-entered Password fields remain plaintext at rest. |
| DAT-02 | Complete export with children, attachment manifests, CLI attachment bytes/checksums, and server-side export permission. See [export](docs/agent/export.md). | Implement the corresponding import and reconciliation path under DAT-01. |
| DAT-03 | Single DocTypes through `isSingle`: one configuration per DocType and instance, with `name: "singleton"` and `docstatus: 0` enforced by PostgreSQL, defaults before the first save, REST/SDK access, permissions on reads and on the first save, and Desk editing at `/app/{doctype}`. See [controller API](docs/agent/controller-api.md). | Export of a Single is not supported, and neither are delete, record rename, submit, cancel and amend. `apps/testapp` declares no Single, so coverage comes from the DocTypes the tests define themselves rather than from a checkout fixture. |
| DAT-04 | Declared renames, conversion safeguards, phased patches, and guarded pruning. See [migrations](docs/agent/migrations.md). | Fixture updates, large backfills, and references embedded in job arguments/history need explicit handling; do not assume renames rewrite those payloads. |
| DAT-05 | Declarative compound business keys through `uniqueKeys`, each a partial unique index, checked before the write and enforced by the database under concurrency. See [field types](docs/agent/fieldtypes.md) and [migrations](docs/agent/migrations.md). | Child tables, `extendDoctype` and a pre-migration duplicate check are out of scope; creating a key over existing duplicates fails with Postgres's own error. Removing `unique: true` from a *field* still leaks its index — the sweep covers only the `uk_` namespace. |
| DAT-06 | Site currency precision/rounding, shared rounding helpers, and site timezone behavior. | Close the database connection timezone and report totals/CSV consistency gaps; report totals currently use ordinary numeric addition. See the [precision and dates design](docs/superpowers/specs/2026-09-10-precisao-decimal-e-datas.md). |
| DAT-09 | Versioned cross-app fields and property overrides through `extendDoctype`. See [extensions](docs/agent/extending.md). | Source customization conversion belongs to migration tooling. A visual customization editor remains demand-driven. |
| PRD-03 | Liveness separated from database readiness, request correlation and duration, queue failure/age signals, operational thresholds, and a `doctor` that reports instead of dying when the database is down. See [operations](docs/agent/ops.md). | Backup alerts wait on recovery automation (PRD-01/02); there is no Prometheus/OTel endpoint, and nothing records the scheduler's last run. |
| PRD-04 | Authorized job inspection, retry, cancellation, retention and per-queue/per-method metrics, over `ddcore jobs`, a System Manager-only HTTP surface and MCP tools; cooperative cancellation of a running job; a daily retention sweep with windows in `ops`; `request_id` carried from the request that queued the work. See [operations](docs/agent/ops.md). | No Desk screen: administration is the CLI and the API. Jobs have no priority and workers no per-queue affinity. A job blocked inside a host call is not interruptible, so cancellation latency is unbounded for it. Payloads are readable only through `ddcore jobs show`, never over HTTP. Nothing is exactly-once: a cancel rolls back the database work and no external effect, and a retry may repeat one. |
| OPS-02 | Business email: file-based templates in `mail/<name>.mail.ts`, a block vocabulary rendered to both parts of the message, `ddcore.sendMail` writing on the caller's transaction, authorized `File` attachments with a size cap, and an `Email Delivery` record per message. The framework's own invitation and recovery messages are two of these templates. See [mail](docs/agent/mail.md). | No inbound mail or IMAP (deferred below); no CC/BCC, Reply-To, per-message From, or resend; attachments must be `File` documents, not raw bytes. A `sensitive` template keeps its arguments out of the delivery record, but they travel in the job payload, so a live token sits in `ddcore_job` until the PRD-04 retention sweep removes it. `Uncertain` records an ambiguous SMTP outcome; it does not resolve it. |
| OPS-03 | Persistent event/date notification rules through `defineNotification`, transactional per-recipient occurrences, authorized Desk inbox and read/unread state, database deduplication, optional template email and recipient-only after-commit SSE invalidation. See [notifications](docs/agent/notifications.md); assignment notifications are delivered through the same inbox (see [assignments](docs/agent/assignments.md), OPS-05). | No visual rule editor, individual preferences, push or custom events. Sent email cannot be withdrawn after access revocation. A date rule resolves recipients once, when its condition first holds for that date. Listing and counting recheck access per stored occurrence, so their cost grows with the inbox; there is no retention sweep for read notifications. |
| OPS-06 | Outgoing webhooks: `Webhook` subscriptions for document lifecycle events and app events (`ddcore.webhooks.emit`), a `Webhook Delivery` record and job written on the caller's transaction, Standard Webhooks signatures with a Vault-held key, a stable `webhook-id` across retries and replay, per-webhook timeout and attempts with exponential job backoff, System Manager replay recorded in `Audit Event`, administration (subscriptions, deliveries, replay) refused to users with access scopes, `DDCORE_WEBHOOKS=off` for rehearsals, retention, `ddcore webhooks` and a doctor section. See [webhooks](docs/agent/webhooks.md). | Delivery is at least once: a timeout is retried, so receivers must deduplicate on `webhook-id`. No per-webhook condition, field selection, custom headers, dual-key secret rotation or private-network (SSRF) restriction; `dbSet` and rename emit nothing. Enabled subscriptions are cached per process: a change made from another process (`ddcore eval --commit`, `exec`, SQL, another server) is not seen by a running server until restart. Users with access scopes cannot administer webhooks at all; there is no scoped webhook administration. A host-blocked HTTP call is not interruptible by job cancellation. |
| OPS-05 | Assignments and pending work: standard `ToDo` DocType in Core, document assignment workflow (`assign`, `complete`, `revoke`), strict referenced-document access filtering on `/api/todo/pending`, due date reminder notifications, Desk sidebar widget and `/app/todo` central. See [assignments](docs/agent/assignments.md). | Assignment does not grant document access; revocation and access changes hide tasks. Pending work loads at most 1000 candidate tasks before the access check, so heavy users can see truncated results. Assignment actions write no audit events, a second open assignment to the same user is not prevented, and assignments are HTTP/Desk only (no server-side API, CLI or MCP tool). Automatic assignment rules remain demand-driven in OPS-09. |
| PRD-06 | Administrative audit coverage: unified audit ledger in `Audit Event` (`tab_audit_event`) absorbing `Vault Audit Log`, engine-level immutability against API/Desk mutations, transactional split (allowed actions committed with transaction, denied attempts written directly to connection pool), recursive sensitive payload redaction, scheduled retention sweep via `ops.auditRetentionDays`, and `ddcore audit list\|purge` CLI. Covers role assign/revoke, user account status/invites/unlocks/sessions, user access scope grant/revoke, workflow transitions (allowed and denied), background job cancel/retry/purge, Vault Secret read/write/delete, webhook replay, and method authorization denials. See [audit](docs/agent/audit.md). | Role changes made via `dbSet`/`setValue` on `User` are not audited. Job administration entries are written outside the caller's transaction, directly on the connection pool. Internal system vault reads during webhook signing bypass auditing to avoid worker log noise. No cryptographic tamper-evident chaining or external SIEM/syslog export. Import and sharing audit events still wait on their respective capabilities (DAT-01, SEC-03). |
| PRD-07 | Core/app compatibility contract: `defineApp({ ddcore: ">=0.14.0 <0.16.0" })` declares the supported core releases (`>=`, `>`, `<=`, `<`, `=`, `^`, `~`); a binary outside the range, an invalid range or a non-semver app `version` refuses the load with every offending app named, on startup, `migrate`, `doctor`, MCP and `dev` hot reload. Builds on a release tag count as that release; `dev`/`latest` builds parse but do not enforce. `doctor` and the export manifest report app versions and ranges; breaking changes are recorded in `CHANGELOG.md`. See [conventions](docs/agent/conventions.md). | Ranges cover core only: `requires` between apps carries no version. No CI job yet runs a representative consumer (ddcore-demo) against a release candidate. The in-source default `engine.Version` is not bumped at release, so an unflagged `go build`/`go test` checks ranges against `0.1.0`. |
| OPS-01 | Document print templates and PDF generation: a standard layout built from DocType metadata, app-declared templates in `print/<name>.print.ts` via `definePrintTemplate` (header, section, keyValues, table, totals, columns, headings, rules, page breaks, raw HTML), the site currency and the print language in values, the core `Letter Head` DocType with a single default, `A4`/`Letter` and landscape written into `@page`, PDF through Gotenberg, a configured command or a local headless Chrome, read permission and user access scopes on the document, Password and Vault fields and fields above the user's permission level removed before rendering (SEC-02), and a Desk preview at `/app/[workspace]/[doctype]/[name]/print` with browser print. See [print templates](docs/agent/print.md). | No running headers, footers or page numbers: the Letter Head prints once before and after the content. `Hidden` fields (as opposed to `permlevel`-restricted ones) reach custom templates, and a template's own `ddcore.db` calls are unfiltered. No audit event for prints. Letter Head HTML is inserted unescaped by design, as System Manager content. Renderer concurrency is fixed (5 for Gotenberg, 3 otherwise). No visual template designer, watermarking or digital signatures. |
| SEC-01 | User access scopes through the Core `User Permission` DocType: per-user `allow`/`for_value` rules, optionally limited to one DocType, enforced below the SDK on lists, counts, link search, direct reads, `db.exists` (by name and by filters), `db.setValue`/`dbSet`, insert/update/delete/submit/cancel/amend, export, attachments, versions/comments, SSE events and notifications, including `Dynamic Link` fields. Two users with identical roles, System Manager included, stay isolated; `ignorePermissions` skips role checks but not scopes; a scoped user, System Manager included, is refused `User Permission` itself, so scope administration belongs to unscoped administrators; `Administrator` and framework-internal elevated contexts are unscoped. Grants and revocations are recorded as `permission.scope_grant`/`scope_revoke` audit events. See [scopes](docs/agent/scopes.md). | `ddcore.db.sql` ignores scopes. Background jobs run with permissions ignored, so a job a scoped user enqueued runs unscoped and app code must filter by scope itself; scheduled methods run as `Administrator`. `is_default` is stored but does not prefill forms, and `allow`/`for_value` are not validated. No test yet covers a report under a scoped user. |
| SEC-02 | Field permissions through `permlevel` on fields and permission rows: unreadable fields omitted from document responses, lists (with filters, sorts, groupings and aggregates on them refused), export, Version diffs, print, per-recipient notification rendering, level-0-only webhook payloads and restricted-field attachments; unwritable changes refused before hooks, with unseen values preserved on save and carried by amendments; child tables judged by the parent; `fieldLevels` in the meta for the Desk; `ddcore.redact` for app code. See [field permissions](docs/agent/field-permissions.md). | Server code stays trusted: `ddcore.getDoc`, `db.getAll`/`getValue`/`sql`, whitelisted method results, script report rows and `ddcore.publish` payloads are unfiltered unless the app calls `ddcore.redact`. A public file uploaded before a field became restricted stays reachable by URL. No permission editor or role profiles (demand-driven under SEC-03). Webhooks cannot opt into restricted fields. Field-level reads and changes are not audited. |
| OPS-04 | Declarative approval workflows through `defineWorkflow`: states, docstatus binding, role-restricted field edits (`allowEdit`), transitions with actions, allowed roles, `allowSelfApproval`, synchronous TypeScript conditions, server-side transition engine with PostgreSQL row locking, `workflow.transition` audit events and timeline comments, and Desk action buttons/state pills. See [workflows](docs/agent/workflows.md). | No pending-approvals inbox or approver notifications. Workflows are linear state machines; parallel/forking approval branches are out of scope. No escalation. |

The authentication mail transport already supported SMTP and a configurable app
transport, and business email (OPS-02) is built on it, so OPS-03 inherits a
delivery record, a template mechanism and a rendering vocabulary rather than
starting from an SMTP client. Outgoing webhooks (OPS-06) added exponential job
backoff and the first `Audit Event` producer, which PRD-06 expanded into unified
administrative audit coverage across framework services. The job worker has retries,
timeouts, leases, heartbeats, and now cancellation and retention (PRD-04), so a durable
delivery service builds on administration that already exists.

Treat the residuals above as bounded follow-up work. Address security or monetary
correctness residuals before deploying a flow that relies on them, and recheck their
status when implementation starts.

## Stage 1 — Broader administrative application support

These capabilities unlock more apps but cost more to design and maintain. Access
controls come first here because subsequent output and sharing features must reuse
their enforcement.

| Capability | Benefit / effort | Dependencies and minimum acceptance |
| --- | --- | --- |
| **Document sharing** — SEC-03 | Medium / M. Supports collaboration beyond static roles. | Define how grants interact with scopes/field restrictions, plus revocation and audit. Test every exposed read path after revoke. Keep permission editors and role profiles in the demand-driven backlog. |

## Stage 2 — Migration and production tooling

This stage follows framework reuse in the default backlog, but its required outcomes
are prerequisites for a production migration. Operational scripts and documented
procedures can deliver value before dedicated screens or CLI products.

| Capability | Benefit / effort | Dependencies and minimum acceptance |
| --- | --- | --- |
| **Resumable import and reconciliation** — DAT-01, PRD-05 | High for migration / L. Makes legacy loading repeatable and reusable. | Build on DAT-02 exports and existing schema migration primitives. Provide mapping, manifest, dry run, per-record errors, checkpoints, and idempotency. Preserve/remap identities, links, children, historical status and metadata through a restricted migration path; avoid replaying historical external effects. Verify attachment bytes/checksums/access and reconcile counts, links, statuses, and monetary totals. Two runs and interrupted resume must not duplicate records, children, or effects. Defer CSV/XLSX UI. |
| **Backup, restore, maintenance, and rollback** — PRD-01, PRD-02 | High for production / M. Establishes recoverability before more operational UI. | Automate database, public/private files, configuration, and version capture; provision secrets separately. Pause writes/jobs for controlled cutovers. Restore into an isolated instance, exercise login and a critical flow, and measure recovery against agreed targets. Define database-compatible rollback and handling of post-cutover writes. |
| **Targeted consumer compatibility** — OPS-11 | Conditional / M–L. Keeps active integrations working without a universal compatibility layer. | Inventory actual API consumers and runtime dependencies. Adapt only required routes, payloads, authentication, errors, and pagination; replace incompatible Python/Node dependencies. Accept through consumer contract tests against pinned source versions. |

## Demand-driven backlog

Promote these only when a concrete app, organization, or measured workload needs
them. Effort and benefit depend on that evidence and must be estimated at promotion.

| Inventory IDs | Deferred capability | Promotion trigger and acceptance focus |
| --- | --- | --- |
| SEC-03 | Permission editors and role profiles | Repeated administration needs; preserve a clear authority for versioned metadata and test effective permissions. |
| SEC-05 | MFA and SSO/OIDC/LDAP | Organization access requirements; implement the actual provider/protocol and test enrollment, recovery, revocation, and session behavior. |
| DAT-01 | CSV/XLSX import UI | Recurring end-user imports; reuse the validated importer and surface row errors and dry-run results. |
| DAT-07 | Trees and virtual DocTypes | Required hierarchies or external entities; validate integrity, queries, links, and permissions for each feature independently. |
| DAT-08 | Rich text and additional field controls | Source data or user workflows need them; prove round-trip fidelity, conversion, and server validation. |
| DAT-09 | Visual customization editor | Demonstrated non-developer authoring demand; define export/versioning and conflict handling before implementation. |
| OPS-07 | Prepared reports and scheduled delivery | Measured report latency/volume; use protected, expiring results and existing jobs/delivery services. |
| OPS-08 | Global search, Calendar, Kanban, and Gantt | Validated navigation/workflow needs; deliver independently and enforce authorization in indexing and results. |
| OPS-09 | Auto Repeat and assignment rules | Two apps require the same semantics; extract common behavior and test repeat execution. |
| OPS-10 | Portals and Web Forms | Required customer/supplier self-service; explicit publication, separate access profiles, abuse controls, and protected attachments. |
| OPS-02 | Inbound email/IMAP inbox | A verified inbound communication workflow; define routing, identity, attachment access, and duplicate handling. |
| PRD-05 | S3-compatible storage, quotas, and retention | Storage scale or lifecycle requirements; verify private access, deletion policy, and recoverability. |
| PRD-08 | Multisite and multiple replicas | Measured deployment/availability need; automate isolated instances first, then validate scheduler, cache, events, and job coordination before same-tenant replication. |

## Delivery and production gates

For each feature, define its public SDK/metadata/API contract in a focused design,
provide server-side enforcement, document it in the agent reference, regenerate
types through supported commands, and cover the acceptance scenarios above.
Run `make test` and applicable `make check` checks; passing current tests does not
prove a future capability exists.

Before a production migration, require a source manifest with pinned versions and
owned critical flows; repeatable and reconciled loading; equivalent required access
controls, approvals, documents, and integrations; restored backups; and rehearsed
cutover/return with explicit recovery targets. Run rehearsals without real outgoing
business effects and keep one system responsible for operational writes. Monitor
errors, queue health, and reconciliation after cutover. Completing every backlog item
is not a prerequisite, but missing a mandatory app requirement is a blocker.

Preserve the architecture described in [CLAUDE.md](CLAUDE.md) and the
[agent reference](docs/agent/index.md): synchronous server TypeScript, Postgres,
file-based structural metadata, English canonical strings with translations, and
one tenant per instance. Accounting, tax, inventory, rental, and other domain rules
belong to apps. New infrastructure must justify its deployment and maintenance cost.
