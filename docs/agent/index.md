# ddcore — reference for agents

ddcore (*Data Driven Core*) is an application framework in the spirit of Frappe: **DocTypes** (models declared
in TypeScript) become Postgres tables, forms, lists and a REST API automatically.
The core is a Go binary (`ddcore`) embedding esbuild + goja: an app's logic is TypeScript
run **on the server, synchronously** (no `await`), and its form scripts run in the desk.

Available documents (also as MCP resources `ddcore://docs/<name>`):

- `conventions` — an app's layout, naming, what never to do, and the core/app compatibility contract (`version`, the `ddcore` range)
- `fieldtypes` — every fieldtype and field property
- `auth` — sign-in, single sign-on (OpenID Connect providers), lockout, recovery, invitation, self-service and secrets
- `field-permissions` — field levels (`permlevel`): confidential fields omitted from every read path and protected on write
- `scopes` — user access scopes (`User Permission`): restricting users to companies, units or customers across every read and write path
- `sharing` — document sharing (`Document Share`): per-user read/write/share grants on one document, scope override, audit
- `controller-api` — `defineController`, hooks, methods, the server's `ddcore.*` API
- `form-api` — `defineForm`, `frm.*`, dialogs, `defineListView` and its Calendar, Kanban, Gantt and Card views (desk)
- `trees` — hierarchical DocTypes (`isTree`): the parent Link and `is_group`, write integrity, the `descendants of` family of filters, scopes down a branch and the Desk tree view
- `search` — global search: the Mod+K palette, which DocTypes are searched (`globalSearch`), ranking and authorization
- `report-api` — `defineReport`, `defineWorkspace`, cards and charts
- `extending` — adding fields to, and overriding properties of, another app's DocTypes
- `mail` — mail templates, blocks, attachments and the delivery record
- `notifications` — persistent event/date rules, authorized recipients, Desk inbox and optional email
- `assignments` — ToDo DocType, assignment workflow, pending work, sidebar widget and due date reminders
- `workflows` — declarative approval workflows: states, actions, docstatus binding, atomic transitions, server enforcement and Desk action buttons
- `webhooks` — outgoing webhooks: subscriptions, signed delivery after commit, retries, replay and the audit record
- `i18n` — English as the source language, catalogues, Select values, dates and the site timezone
- `migrations` — renames, fieldtype changes, patches and the expand → contract route
- `upgrade-0.17` — moving an app across the 0.17 key rename (`name` → `id`): what to rename, what fails silently, how to find it
- `storage` — where uploaded file bytes live (local or S3-compatible), download access and deletion
- `backup` — `ddcore backup`/`restore`, maintenance mode, the site version ledger and rollback
- `export` — exporting a whole DocType, children and attachments, by HTTP or CLI
- `import` — loading an export into a site: identities and metadata kept, effects not replayed, resumable, reconciled
- `data-import` — loading a CSV/XLSX file from the Desk or HTTP: each row a normal insert or update as the user, with a dry run and per-row errors
- `print` — print templates, print block builders, Letter Head branding, and server-side PDF generation
- `external-db` — `ddcore.externalDb(name)`: read-only SQL Server queries from server code, configured from `DDCORE_SECRET_<NAME>_*`
- `vault` — encrypted credential vault (`ddcore.vault.*`), `Vault` fieldtype, and audit logging
- `audit` — unified administrative audit events (`tab_audit_event`), sanitization, immutability, retention and CLI inspection
- `cli` — the `ddcore` commands and the development loop
- `ops` — liveness and readiness probes, request correlation, queue signals and `doctor`
- `feature-requests` — when a gap belongs in the framework, and how to report it upstream

What changed between releases is not in this list — it is the changelog, served as its own
resource `ddcore://changelog`. The `whats_new` tool returns the part of it above the version you
are running, together with the newest published release; `ddcore doctor` warns when that release
is ahead of your binary. Read it after an upgrade, and before reporting a gap.

## Typical flow

1. In a monorepo, `ddcore new-app <name>` creates `apps/<name>`; in an app's own
   repository, use `ddcore new-app <name> --dir .` and `apps: ["."]`.
2. Write `doctypes/<snake>/<snake>.doctype.ts` with `defineDoctype`.
3. `ddcore migrate` (or the MCP `migrate` tool) creates and alters the tables and generates,
   under `.ddcore/`, the DocType types and the declarations of the embedded SDKs.
4. Rules in `<snake>.controller.ts`; screen scripts in `<snake>.form.ts`; tests in `<snake>.test.ts`.
5. `ddcore dev` runs the server with hot reload; `ddcore test` runs the tests in rolled-back transactions.

## Mental model

- Single DocTypes (`isSingle`) expose one settings document with a fixed `singleton` identity; see `controller-api`.
- One DocType = one table `tab_<snake_case>`; child tables (`isChild`) have `parent`, `parenttype`, `parentfield`, `idx`.
- Standard columns: `id` (PK, text), `owner`, `creation`, `modified`, `modified_by`, `docstatus` (0 draft, 1 submitted, 2 cancelled). The key is `id` (`doc.id`, filters and `fields` on `id`); `name` is an ordinary fieldname an app may declare. A pre-0.17 database is moved from `name` to `id` by `migrate` — see `migrations`; an app written before 0.17 is moved by `upgrade-0.17`.
- Lifecycle: `beforeValidate → validate → beforeSave → (insert|update) → afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel → onCancel`; `onTrash → afterDelete`.
- The core validates `reqd`, `unique`, `uniqueKeys` (compound business keys, checked before the write *and* enforced by a partial unique index, so a race cannot slip through), a Select's `options`, that links exist, `fetchFrom`, `mandatoryDependsOn` (**on the server**) and refuses to change a field without `allowOnSubmit` once submitted.
- One transaction per request or job. An error rolls it back. There is no `commit()` for an app.
- A rename is **declared** (`renamedFrom`), never inferred, and a fieldtype change that could lose data is refused until you declare `convert` or move the data across releases. See `migrations`.
- **Every user-facing string is English and is a key** — a `label:` as much as a `_("…")`. Translations live in `translations/<lang>.csv`, and a Select's value is canonical English with a translated label. See `i18n`.
