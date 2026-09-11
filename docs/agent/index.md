# ddcore — reference for agents

ddcore (*Data Driven Core*) is an application framework in the spirit of Frappe: **DocTypes** (models declared
in TypeScript) become Postgres tables, forms, lists and a REST API automatically.
The core is a Go binary (`ddcore`) embedding esbuild + goja: an app's logic is TypeScript
run **on the server, synchronously** (no `await`), and its form scripts run in the desk.

Available documents (also as MCP resources `ddcore://docs/<name>`):

- `conventions` — an app's layout, naming, what never to do
- `fieldtypes` — every fieldtype and field property
- `auth` — sign-in, lockout, recovery, invitation, self-service and secrets
- `controller-api` — `defineController`, hooks, methods, the server's `ddcore.*` API
- `form-api` — `defineForm`, `frm.*`, dialogs (desk)
- `report-api` — `defineReport`, `defineWorkspace`, cards and charts
- `extending` — adding fields to, and overriding properties of, another app's DocTypes
- `i18n` — English as the source language, catalogues, Select values, dates and the site timezone
- `migrations` — renames, fieldtype changes, patches and the expand → contract route
- `export` — exporting a whole DocType, children and attachments, by HTTP or CLI
- `cli` — the `ddcore` commands and the development loop
- `ops` — liveness and readiness probes, request correlation, queue signals and `doctor`

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
- Standard columns: `name` (PK, text), `owner`, `creation`, `modified`, `modified_by`, `docstatus` (0 draft, 1 submitted, 2 cancelled).
- Lifecycle: `beforeValidate → validate → beforeSave → (insert|update) → afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel → onCancel`; `onTrash → afterDelete`.
- The core validates `reqd`, `unique`, `uniqueKeys` (compound business keys, checked before the write *and* enforced by a partial unique index, so a race cannot slip through), a Select's `options`, that links exist, `fetchFrom`, `mandatoryDependsOn` (**on the server**) and refuses to change a field without `allowOnSubmit` once submitted.
- One transaction per request or job. An error rolls it back. There is no `commit()` for an app.
- A rename is **declared** (`renamedFrom`), never inferred, and a fieldtype change that could lose data is refused until you declare `convert` or move the data across releases. See `migrations`.
- **Every user-facing string is English and is a key** — a `label:` as much as a `_("…")`. Translations live in `translations/<lang>.csv`, and a Select's value is canonical English with a translated label. See `i18n`.
