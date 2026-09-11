# Development: ddcore + the `apps/demo` app

This document explains **how work is done** in this repository: what ddcore is, what the demo
app is, where the boundary between the core and an app runs, which files exist for what, and
what the day-to-day loop looks like.

There is a peculiarity here that a product app does not have: this checkout is the
**framework's monorepo**. The binary is not installed — it is compiled from here
(`bin/ddcore`) — and `apps/demo` is loaded by this very `ddcore.json`. So there are almost
always **two loops** running at once: the core's (Go + desk) and the app's (TypeScript).

Alongside:

- `README.md` — overview, setup and the monorepo's layout.
- `CLAUDE.md` / `AGENTS.md` — the short, non-negotiable rules (synchronous, ports, `.ddcore/`).
- `docs/plan-v1.md` — the framework's design (in Portuguese; historical).
- `docs/agent/` (`ddcore docs`, or the MCP resources `ddcore://docs/*`) — **the canonical API
  reference**: `conventions`, `fieldtypes`, `controller-api`, `form-api`, `report-api`,
  `i18n`, `migrations`, `cli`. This document describes the mental model; it does not
  replace the API.
- `docs/superpowers/specs/` — the specs behind what is built here.

*Em português: [`DEVELOPMENT.ptbr.md`](DEVELOPMENT.ptbr.md).*

---

## 1. What ddcore is

ddcore (*Data Driven Core*) is a **data application framework** in the spirit of Frappe,
implemented in **Go + TypeScript + PostgreSQL** and shipped as a single binary (`ddcore`). It
is not a library an app imports and compiles against: it is a **host**. The binary embeds
esbuild and goja, the HTTP server, the database layer, the desk (a Svelte 5 SPA) and the SDK
typings.

An app is a directory of TypeScript files the binary loads and runs. Hence the most important
practical consequence:

> **An app has no build of its own, no server of its own, and no dependency on Node in
> production.** The only engine is the `ddcore` binary. The only external dependency is a
> Postgres.

An app's server code runs inside the **goja** interpreter, which is **synchronous**: there is
no event loop, no useful `Promise`, no `await`. I/O (database, HTTP, cache) is a synchronous
call that blocks and returns the value. It is a design choice of the framework — and the
reason for the rule "never `await` on the server".

### The three SDKs

| Module | Where it runs | What it is |
|---|---|---|
| `@ddcore/sdk` | the server (goja), **synchronous** | `defineDoctype`, `defineController`, `defineReport`, `defineWorkspace`, `defineApp`, `whitelisted`, and the `ddcore` global (db, utils, http, cache, log…) |
| `@ddcore/sdk/test` | `ddcore test` | `describe`/`it`/`expect` |
| `@ddcore/desk-sdk` | the browser, **asynchronous** | `defineForm`, `defineListView`, `ddcore.ui.Dialog`, `ddcore.db.*` (over HTTP), `ddcore.format`, `ddcore.datetime` |

**One difference specific to this repository.** In an outside app the three are *materialised*
by `ddcore types` under `.ddcore/`, and `tsconfig.json` points there. Here, because the SDK
sources are in the checkout itself, `apps/demo/tsconfig.json` resolves `@ddcore/sdk`,
`@ddcore/sdk/test` and `@ddcore/desk-sdk` straight into `packages/` — the app's typecheck runs
against the SDK **source**, not a generated copy. Changing `packages/sdk` breaks (or fixes) the
app's typecheck immediately, which is exactly the effect wanted.

What `./bin/ddcore types` still generates under `apps/demo/.ddcore/` is the **DocType types**
(`types.d.ts`), the desk entry point and the embedded declarations. `.ddcore/` is gitignored
and must **never** be edited by hand.

---

## 2. Division of responsibilities

The general rule: **the core solves what is generic to any data app; an app describes and
decides what is specific to its domain.** In the monorepo each core responsibility has a
directory of its own.

### What ddcore (the core) does — and where it lives

| Responsibility | Where |
|---|---|
| The meta-model: interprets DocTypes and validates the meta | `internal/meta` |
| DDL: creates and alters `tab_<snake>`, columns, indexes; declared renames and conversions; migrate, patches, fixtures | `internal/db` |
| Runs the app's TypeScript (esbuild + goja), hot reload | `internal/js` |
| `Document` (insert/save/submit/cancel/delete), hooks, permissions, jobs and queues | `internal/engine` |
| The REST API, `/api/method`, meta, reports, SSE, upload, login/CSRF | `internal/api` |
| The MCP server (tools and resources) | `internal/mcp` |
| Scaffolding (`ddcore init`, `new-app`, `scaffold_doctype`) | `internal/scaffold` |
| Type generation (`ddcore types`) | `internal/typegen` |
| Extracting translatable strings (`ddcore i18n extract`) | `internal/i18nx` |
| The embedded app: User, Role, Has Role, File, Comment, Version, Error Log, API Key | `core/` |
| The desk: lists, forms, grids, filters, workspaces, reports, dialogs, i18n | `desk/` |
| The SDKs an app imports | `packages/sdk`, `packages/desk-sdk` |
| The CLI | `cmd/ddcore` |

In behavioural terms, the core is what guarantees:

- **The lifecycle** — `beforeValidate → validate → beforeSave → (insert|update) →
  afterInsert/onUpdate`; `beforeSubmit → onSubmit`; `beforeCancel → onCancel`;
  `onTrash → afterDelete`. `docstatus` 0/1/2, naming, `amended_from`, child tables
  (`parent`/`parenttype`/`parentfield`/`idx`).
- **Structural validation, on the server** — `reqd`, `unique`, a Select's `options`, that
  links exist, `fetchFrom`, `mandatoryDependsOn`, and refusing to change a field without
  `allowOnSubmit` once submitted.
- **Transactions** — one per request, job or test `it`. An error rolls it back. An app has no
  `commit()`.
- **Permissions** — the per-role `permissions`, plus the controller's `hasPermission` and
  `permissionQuery` hooks.
- **The whole desk** — an app **writes no UI components**; it describes fields and, where it
  has to, adjusts behaviour by script.
- **Language** — resolving the reader's language, translating errors and metadata at the HTTP
  border, and the regional formatting of dates, numbers and money.
- **The scheduler and the queues** — running the `scheduler` an app declares, and
  `ddcore.enqueue`.
- **The tooling** — `dev`, `migrate`, `test`, `types`, `i18n`, `exec`, `eval`, `demo`, `jobs`,
  `user`, `apikey`, `doctor`, `docs`, `mcp`.

### What the app (`apps/demo`) does

The demo is deliberately small: projects, milestones and tasks. It exists to be an
**executable tutorial, an end-to-end fixture and a reference for good practice** — not to
carry a real product's rules. Within that scope it:

- **declares the domain**: the `Project`, `Task` and `Project Milestone` (child) DocTypes,
  with the `Project Manager` and `Project Contributor` roles;
- **writes the rules** in its controllers and services: date-range validation, the idempotent
  `start`/`complete`/`reopen` transitions, recalculating a project's progress, marking overdue
  tasks daily;
- **declares navigation and reading**: the `Projects` workspace (sidebar, shortcuts, cards,
  chart) and the `Tasks by Status` report;
- **adjusts the UI at the edges**: `*.form.ts` (buttons, indicators, a Link's filter) — and
  `client/lists.ts` shows the opposite: with `optionColors` on the status field, a list needs
  no custom indicator at all;
- **declares the periodic routine** in `ddcore.app.ts` (`daily: demo.services.tasks.markOverdue`)
  — the core is what runs it;
- **seeds a demonstration** in `services/demo.ts`, idempotent, discovered by `ddcore demo`;
- **covers all of it** with `*.test.ts`.

Being an example, it also **deliberately avoids** things the framework supports: external
integrations, attachments, `hasPermission`/`permissionQuery`, raw SQL, and patches with no
real migration to demonstrate. When a heavier case is needed, the core's own tests cover it —
do not push the example to grow.

The one exception is `Invoice`, and it names the rule it is an exception to: decimal precision
is a **framework contract**, not an app feature. An author has to be able to read a worked case
of a Currency stored at the site's precision and a schedule of instalments that adds back up to
its total. The exhaustive matrix still lives in the core's tests; the example carries the one
case someone will copy.

### The boundary, one line per case

| You need… | Who solves it |
|---|---|
| a new column in the database | the core — declare the field in `.doctype.ts` |
| `completed_on` required when `completed` | the core, through `mandatoryDependsOn` (desk **and** server) |
| stopping a due date before the project starts | the app — the controller's `validate` |
| a "Complete" button on the form | the app declares it (`frm.addButton`), the core renders it |
| marking tasks overdue every day | the app declares it under `scheduler`, the core runs it |
| writing a derived field without re-entering the hooks | the core provides `dbSet`, the app decides when |
| formatting a percentage in the list | the core, from `fieldtype: "Percent"` |
| deciding that progress = completed/total | the app — `services/projects.ts` |
| showing a status in the reader's language | the core — the value is canonical English and its label comes from the catalogue |

---

## 3. Anatomy of the repository

```
cmd/ddcore/                   CLI
internal/                     meta · db · js · engine · api · mcp · scaffold · typegen · i18nx
core/                         the embedded app (User, Role, File, Version, …)
desk/                         SvelteKit, its build embedded in the binary
packages/sdk/                 @ddcore/sdk
packages/desk-sdk/            @ddcore/desk-sdk
docs/agent/                   the API reference, written for agents
ddcore.json                   what the site decided: apps, currency, timezone, access policy
.env / .env.example           where it is running: database, port, public URL, mail
Makefile                      shortcuts for the working loop
bin/ddcore                    the compiled binary (produced by make build)
apps/demo/                    the demo app
```

Configuration is split by the question it answers, and the split is not
cosmetic. `ddcore.json` holds what the site decided — its apps, currency and
precision, timezone, and the access policy under `auth` — so it is committed
and identical on a laptop, in staging and in production; staging has to lock an
account out exactly like production does, or the rehearsal proves nothing.
`.env` holds where the site is running — the database, the port, the public
URL, and everything about outgoing mail, one field of which is a password — so
it is ignored by version control, and `.env.example` is the committed record of
which variables exist. Precedence runs outwards: a variable already in the real
environment beats `.env`, which beats `ddcore.json`, which beats the default.
That order is what lets Railway inject `DATABASE_URL`, or `docker run -e`
override a port, without anyone editing a file inside the image.

### Inside `apps/demo`

```
ddcore.app.ts                   the manifest: name, roles, scheduler, desk
tsconfig.json                   resolves @ddcore/sdk into packages/sdk/src
doctypes/<name>/
  <name>.doctype.ts             meta: fields, permissions, naming          [server, declarative]
  <name>.controller.ts          lifecycle hooks and methods                [server, synchronous]
  <name>.form.ts                the form's behaviour in the desk           [browser, async]
  <name>.test.ts                tests                                      [ddcore test]
services/*.ts                   reusable business rules                    [server, synchronous]
client/lists.ts                 global desk scripts                        [browser, async]
reports/*.report.ts             reports                                    [server, synchronous]
workspaces/*.workspace.ts       navigation and dashboard                   [server, declarative]
translations/pt-BR.csv          translations, generated by `ddcore i18n extract`
.ddcore/                        GENERATED by `ddcore types` — do not edit
```

The naming convention **is** the discovery mechanism: the binary walks the app's directory and
registers by suffix (`.doctype.ts`, `.controller.ts`, `.form.ts`, `.report.ts`,
`.workspace.ts`, `.test.ts`). There is no index file to keep in step.

### A DocType's four files

Taking `Project` as the example:

**1. `project.doctype.ts` — the meta.** Purely declarative. Fields with a `fieldtype` (`Data`,
`Link`, `Select`, `Percent`, `Table`…), `reqd`, `unique`, `readOnly`, `inListView`,
`inStandardFilter`, `naming: { field: "code" }`, `titleField`, `trackChanges`. From this the
core derives the Postgres schema, the form's layout, the list's columns, the filters and the
typings in `.ddcore/types.d.ts`.

```ts
export default defineDoctype({
  name: "Project",
  module: "Projects",
  naming: { field: "code" },
  titleField: "title",
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true, inListView: true },
    { fieldname: "assignee", fieldtype: "Link", label: "Assignee", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "progress", fieldtype: "Percent", label: "Progress", readOnly: true },
    { fieldname: "milestones", fieldtype: "Table", label: "Milestones", options: "Project Milestone", gridEditMode: "inline" },
    // ...
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Project Contributor", read: true },
  ],
});
```

Note `status` and `progress`: they are `readOnly` because they are **derived**. A derived
field is never typed; the service maintains it, through `dbSet`.

Note the labels too: they are English because a label is a catalogue key. The accents live in
`translations/pt-BR.csv`.

**2. `project.controller.ts` — the rules.** Synchronous. `validate` guards the invariant and
runs on **every** write, whether it comes from the desk, the API, a job or a test — which is
why validation lives here and not in the form. `methods` exposes actions the desk can call
(`start`, `complete`, `reopen` on Task), which save through the normal lifecycle and are
idempotent. Use `ddcore.throw` with a `title` for a business error, `doc.dbSet` to write a
derived column without re-entering `validate`, and `doc.getDocBeforeSave()` when the effect
depends on the previous value (moving a task between projects recalculates **both**).

**3. `project.form.ts` — the UI.** Runs in the browser and **may** be asynchronous.
Indicators, buttons conditional on state, `frm.setQuery` to restrict a Link, `frm.call(...)`
to invoke a controller method and reload. Only convenience lives here: nothing the server has
to guarantee may depend on this file.

**4. `project.test.ts` — the safety net.** Each `it` runs in a rolled-back transaction.

### Services: where the domain actually lives

A good controller is thin. The logic goes in `services/*.ts`, because it has to be called from
several places with the same meaning: from `validate`, from a controller method, from a
report, from a workspace card, from the scheduler, from `ddcore exec` and from the tests. The
demo shows this three times:

- `services/projects.ts:recalculateProgress` — called after inserting, updating or deleting a
  task;
- `services/tasks.ts:markOverdue` — called by the daily scheduler **and**, through the
  `markOverdueNow` wrapper (`whitelisted`, restricted to `Project Manager`), by a desk button;
- `services/tasks.ts:summaryByStatus` — shared by the report **and** by the workspace chart,
  so the two counts cannot drift apart.

Functions marked `whitelisted(...)` get an HTTP endpoint
(`POST /api/method/demo.services.tasks.markOverdueNow`) — that is how the desk calls the
server outside a document's context.

Interface text is written as an English key and translated in `translations/pt-BR.csv`
(`Start`, `Complete`, `Reopen`, `Open`, `In progress`, `Overdue`, `Completed`). A Select's
value *is* its key: `_(row.status)` and nothing else. See `docs/agent/i18n.md`.

---

## 4. The development loop

### Setup (once)

```bash
make docker-up                               # dev Postgres (container ddcore-pg, port 5455)
make build                                   # desk (npm) + the binary at bin/ddcore
make migrate                                 # the core's DDL + installing the demo app
./bin/ddcore user passwd Administrator admin
./bin/ddcore demo                            # demonstration data (idempotent)
make dev                                     # http://localhost:8090
```

Requirements: Docker, Go and Node.js. The Go tests use the `ddcore_test` database (recreated);
`DDCORE_DSN` and `DDCORE_TEST_DSN` override the DSN of any command.

### The two loops

**Working on the app (`apps/demo/**/*.ts`).** `make dev` keeps running and rebuilds on save:
no restart, no rebuild. If the change touched a DocType's **meta**, run `./bin/ddcore migrate`
(or let `dev`'s `--auto-migrate` handle it) and `./bin/ddcore types` to regenerate the
typings. If it touched a string, run `./bin/ddcore i18n extract --all --lang pt-BR` and fill
in the new rows — `make check` fails otherwise.

**Working on the core (`internal/`, `core/`, `cmd/`, `packages/`, `desk/`).** Now the binary
has changed: `make build` (or `go build -o bin/ddcore ./cmd/ddcore` when the desk did not
change) and restart `dev`. A change in `packages/sdk` shows up in the app's typecheck
immediately, because the demo's `tsconfig.json` points at the source.

In both cases, when 8090 is taken use `make stop` rather than starting a second server.

### The commands that matter

| Command | For what |
|---|---|
| `make build` | desk + `bin/ddcore` |
| `make dev` / `make stop` | the server on :8090 with hot reload |
| `make migrate` | DDL + `afterInstall` + fixtures + patches + `afterMigrate` + types |
| `make check` | the desk's `svelte-check`, `ddcore types`, `tsc` for `apps/demo`, and the translation catalogue |
| `make i18n` | rewrites `translations/<lang>.csv` from the code |
| `make test` | build + `go vet` + the Go tests + `ddcore test --app demo` + the desk tests |
| `./bin/ddcore test --filter <regex> -v` | iterating on one app test |
| `./bin/ddcore demo` | seeds demonstration data (idempotent) |
| `./bin/ddcore exec demo.services.tasks.markOverdue` | runs a service outside a request |
| `./bin/ddcore eval '<ts>' [--commit]` | loose TS with `ddcore.*` (rolls back by default) |
| `./bin/ddcore doctor` | database, meta, pending DDL, scheduler |
| `./bin/ddcore docs` / MCP `ddcore://docs/*` | the API reference |
| `make docker-psql` | psql on `ddcore_dev` |

`make test` is the definition of done — it includes `internal/acceptance`, which checks
installation, boot, translation and `/app` against a real Postgres over real HTTP.

### Non-negotiable rules

- The server is **synchronous**: no `await`/`Promise` in `*.controller.ts`, `services/`,
  `reports/`, `workspaces/`, `patches/`.
- The desk (`*.form.ts`, `client/*.ts`) **may** be asynchronous — and almost always is.
- **Never** edit `apps/demo/.ddcore/`; run `./bin/ddcore types`.
- An invariant is validated on the server (`validate`), never only in the form.
- Interface text is English, goes through `_()` / `__()` (or a `label:`), and has a row in the
  translation CSV. `make check` enforces it.
- No app file imports source by a relative path outside `apps/demo`: the SDKs come in only as
  `@ddcore/sdk` and `@ddcore/desk-sdk`.
- The demo app uses no raw SQL; queries go through `ddcore.db.getList` and friends.
- Use the MCP tools rather than touching the database by hand.
- `make test` before calling anything done.

---

## 5. Reading one flow end to end

"Complete a task" crosses every layer and serves as a map:

1. **Desk** — `doctypes/task/task.form.ts` shows the `Complete` button according to the state
   and calls `frm.call("complete")`.
2. **Core** — receives `POST /api/resource/Task/:name/complete` (`internal/api`),
   authenticates, checks permission, opens the transaction, loads the document
   (`internal/engine`) and invokes the controller's method in goja (`internal/js`).
3. **Controller** — `task.controller.ts:methods.complete` is idempotent: if it is already
   completed it returns the current state; otherwise it sets `status = "Completed"`,
   `completed_at = ddcore.utils.now()` and saves through the normal lifecycle.
4. **Hook + service** — Task's `onUpdate` calls
   `services/projects.ts:recalculateProgress(project)`, which counts completed tasks and
   writes the Project's `progress` and `status` with `dbSet` (no hook recursion).
5. **Core** — on success, commit and `{ status, completed_at }` in the response; on
   `ddcore.throw`, rollback and a business message on screen — translated at the border into
   the reader's language.
6. **Desk** — reloads the document; the indicator and the progress come back updated, and the
   workspace's card and chart show the same count because they read the same service.

The same path is exercised with no UI at all by `doctypes/task/task.test.ts` and
`services/tasks.test.ts`. That is the shape of the framework: **the core moves the document
through the lifecycle and the transaction; the app says what is true about the domain at each
point along the way.**
