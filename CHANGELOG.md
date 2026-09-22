# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

The sections for 0.1.0 through 0.14.0 were written after the fact, from the repository's history.
They record what an app would notice in each release — a capability, a contract, a default — and
not every commit that went into it.

## Unreleased

### Added

- Tree DocTypes (DAT-07): `defineDoctype({ isTree: true })` makes a DocType a hierarchy. It gets a
  self-referencing Link — `parent_<snake(name)>`, or the `parentField` you name — and an `is_group`
  Check, both added unless you declare them yourself. The engine keeps the hierarchy honest on
  every write: the parent must exist and be a group, a document cannot be moved under itself or
  under one of its own descendants, a group with children stays a group, and deleting a document
  that still has children is refused with a message that says so (and is not waived by `force`).
  `dbSet` is checked too when it writes either column. Structural writes on one tree DocType are
  serialised by an advisory lock, so two concurrent moves cannot weave a cycle between them.
  See [trees](docs/agent/trees.md).
- Tree filter operators, usable anywhere filters are — `getList`, REST, reports, the Desk:
  `descendants of`, `descendants of (inclusive)`, `not descendants of`, `ancestors of` and
  `not ancestors of`, over a tree's own `id` or over a Link pointing at one. They compile to a
  recursive query and compose with permissions, scopes and shares like any other filter.
- A User Permission whose `allow` is a tree DocType now covers the branch below the value it names
  (SEC-01): "Territory: Brazil" grants Brazil and everything under it, on lists, counts, link
  search, direct reads and writes. See [scopes](docs/agent/scopes.md).
- `GET /api/tree/{doctype}?parent=&limit=` returns one level of a hierarchy with each node's
  readable child count, and the Desk's new **Tree** view is built on it: it is the default view for
  a tree DocType, expands a branch at a time, links each node to its form and offers "Add child" on
  a group. A node whose parent the user cannot read is shown as a root.
- `ddcore import` loads a `ddcore export` directory into a site (DAT-01): `plan`, `run`,
  `status` and `reconcile`, with `--dry-run`, `--resume`, `--batch`, `--only`, `--include`,
  `--max-batches`, `--maintenance` and `--map`, plus an `import` MCP tool. A load keeps ids,
  owners, timestamps and docstatus (including cancelled documents), runs no controller hook and
  queues no webhook, notification, email or realtime event; it advances `ddcore_series` past the
  ids it writes and marks already-past date notifications as done. Every line leaves a ledger
  row, so a second run over the same directory writes nothing and an interrupted run resumes at
  the first line that did not commit. Attachments are verified against the export's checksums
  before their bytes are written. `reconcile` compares rows, child rows, docstatus, files and
  per-docstatus Currency totals, and exits non-zero on any disagreement. A mapping file renames
  DocTypes and fields, drops, sets constants and remaps ids and users. See
  [import](docs/agent/import.md).

### Changed

- `ddcore export --attachments` writes the attachment bytes to `<out>/files/public/<file>` and
  `<out>/files/private/<file>` — their storage key — instead of `<out>/files/<file>`. A public and
  a private file sharing a base name no longer overwrite each other. `manifest.json` records the
  layout in `exportFormat`.

### Fixed

- Checkbox fields in the Desk now render their `description` helper text below the label (with proper left alignment), as well as field validation errors and required asterisks.
- `descendants of` was accepted as a filter operator and compiled into `=`, which answered a
  hierarchy question with an exact match — a wrong result, with no error. It now walks the tree,
  and naming it on a field that is neither a tree's `id` nor a Link to a tree is refused.

## 0.17.0 — 2026-09-22

### Breaking

- The document key is now `id` instead of `name`, everywhere: the column, filters, `fields`,
  `orderBy`, `titleField`/`searchFields`, `BaseDoc.id` in the SDK, `doc.id` in controllers and
  form scripts, the REST routes (`/api/resource/{doctype}/{id}`, `/api/print/…/{id}`,
  `/api/comments|versions|assignments|shares/{doctype}/{id}`, `PATCH /api/notifications/{id}`),
  the request bodies of assignments, shares and workflow (`{ doctype, id, … }`,
  `?doctype=…&id=…`), the rename body (`{ "id": … }`), link titles (`?ids=`), the upload field
  (`doc_id`), global search hits, the realtime `doc_update` payload, the webhook envelope's
  `data.id`, and the MCP tools `get_doc`, `update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`
  and `call_method`. The desk's document route is `/app/<workspace>/<doctype>/<id>`. There is no
  alias: a filter, field list or SQL that still says `name` fails on an unknown field or column,
  but `doc.name` reads `undefined` without an error, and `tsc` does not flag it.
  On an existing database the next `ddcore migrate` renames the column on every DocType table
  before anything else runs, so every patch — `beforeSchema` or `afterSchema` — sees `id`; primary
  keys, Single checks and indexes follow on their own, and `migrate --dry-run` previews it. App
  code must say `id` wherever it meant the key: `doc.id`, `ddcore.getDoc(doctype, id)`,
  `filters: { id: … }`, `fields: ["id"]`, `"count(id) as n"`, raw SQL in patches and reports.
  `name` is now an ordinary fieldname an app may declare; declaring it does not bring the old key
  back. Update apps' `ddcore:` range to `>=0.17.0` once they have been through their code and
  their patches, run or not: a 0.17 binary warns about any app whose range still reaches below it.
  The step-by-step checklist, including what fails silently, is `docs/agent/upgrade-0.17.md`
  (`ddcore://docs/upgrade-0.17`).
- The fields that held another document's key follow: `reference_name` → `reference_id`
  (Comment, ToDo, Email Delivery, Webhook Delivery, notifications), `share_name` → `share_id`
  (Document Share), `attached_to_name` → `attached_to_id` (File), `target_name` → `target_id`
  (Audit Event), and Version's `docname` → `doc_id`. `migrate` renames them, their indexes
  included; filters and reports that name them must use the new names.
- The vocabulary around the key follows it: `naming` on a DocType is `idGeneration` (same
  `series`, `field`, `hash`, `prompt`, `format`), the `naming_series` field is `id_series`,
  `nameLabel` is `idLabel`, and `defineListView(…, { nameColumn: false })` is
  `{ idColumn: false }`. The list's leading column is headed "ID" by default. A Vault field's key
  template says `{id}` where it said `{name}`; a template still saying `{name}` is refused at load
  unless the DocType declares a field called `name`. Keys already stored in the vault are values
  and do not move.
- `ddcore export` (NDJSON and CSV) writes the key as an `id` column, and the attachment manifest's
  `name` is `id`; a consumer of those files must read the new column. A backup taken before 0.17
  restores as it was and is brought up by the migrate `ddcore restore` runs; with `--no-migrate`,
  `--smoke` now says to run `ddcore migrate` instead of failing on the column.

## 0.16.0 — 2026-09-22

### Breaking

- The superuser is now `Admin` instead of `Administrator`. A new database gets `Admin`, and on an
  existing one the next `ddcore migrate` renames the user in place, with its roles, sessions,
  tokens, audit entries, `owner`/`modified_by` columns and every Link to it. Sign in as `Admin`
  with the same password. App code that names the user — `c.User == "Administrator"`, a test that
  signs in as it, a fixture that links to it — must say `Admin`. The rename is skipped when both
  users already exist.

### Added

- `ddcore migrate` gives `Admin` a generated 10-character password (lowercase, uppercase, digit and
  symbol; longer when `auth.minPasswordLength` asks for more) when it has none, which is what a
  first install leaves, and prints it once to stderr. `dev`/`start --auto-migrate` and `restore`
  print it the same way, and the MCP `migrate` tool returns it as `adminPassword`. Only the hash is
  kept, and a password Admin already has is never replaced. The step
  `ddcore user passwd Admin <password>` is no longer needed to sign in the first time.
- `defineListView(doctype, { nameColumn: false })` hides the leading document-name column of a
  list, for a DocType whose name means nothing to a reader, such as a `hash`. The row stays
  clickable. The default is unchanged. ([#7](https://github.com/jrvidotti/ddcore/issues/7))
- `nameLabel` on a DocType names its identifier: `nameLabel: "Contract No."` heads the list's name
  column instead of "Name", and labels the name when it is prompted for or renamed. It is a
  catalogue key, so `ddcore i18n extract` collects it and an extension may override it. It changes
  display only: filters, `orderBy` and the API still say `name`.
  ([#7](https://github.com/jrvidotti/ddcore/issues/7))

### Changed

- When a list has no name column, the title field's cell links to the document, so it can be
  opened in a new tab.

## 0.15.1 — 2026-09-18

### Added

- `ddcore init` writes a `docker-compose.yml` that runs PostgreSQL with the user, password,
  database and port in the DSN, so `docker compose up -d` gives a new project its database.
  It writes the file only for a DSN on this machine, and leaves an existing `compose.yaml`,
  `compose.yml`, `docker-compose.yaml` or `docker-compose.yml` alone.
- `ddcore init --name <n> --db-port <p>` builds the DSN
  `postgres://n:n@localhost:p/n?sslmode=disable`, so you no longer have to type it out. `--dsn`
  still points at an existing database, and it cannot be combined with the two new flags.
- `ddcore init` also writes the project's `README.md` (setup, commands, MCP), `AGENTS.md` with
  the conventions for coding agents (`CLAUDE.md` is a symlink to it), a `.mcp.json` that
  registers `ddcore mcp`, and a `.gitignore` that keeps `.env`, `.ddcore/` and `data/` out. It
  never overwrites an existing file. Its closing message lists every step up to
  `ddcore user passwd Admin` and the URL.
- `ddcore new-app` writes `version: "0.1.0"` and a `ddcore` range for the running minor release
  (`>=0.15.0 <0.16.0` on 0.15.x) into `ddcore.app.ts`. A build that is not a release leaves the
  range commented out.

### Changed

- Without `--dsn` or `--name`, `ddcore init` now names the database, user and password after the
  directory (for example `my-shop` becomes `my_shop`) instead of `ddcore`.
- `ddcore new-app` no longer writes `apps/<app>/CLAUDE.md`. The guide now lives once, at the
  project root, as `AGENTS.md`. Existing apps keep their file.

### Fixed

- The `ddcore running` log line reports the configured public `url` instead of always
  `http://localhost:<port>`; the local address moved to a `listen` field.

## 0.15.0 — 2026-09-17

### Added

- Release awareness: this file now ships inside the binary and is served as the MCP resource
  `ddcore://changelog`. The `whats_new` tool returns the part of it above the running version,
  together with the newest published release.
- `ddcore doctor` asks GitHub for the newest published release and warns when the binary is
  behind it, so `--strict` fails on a stale one. It asks at most once an hour, stays silent when
  it cannot reach GitHub or when the binary is not a release, and is skipped by
  `--no-update-check` for one run or `DDCORE_UPDATE_CHECK=off` for every run.

- Backup, restore and maintenance (PRD-01, PRD-02):
  - `ddcore backup` writes one checksummed archive of the database, stored files, configuration
    and versions, with optional S3 upload and retention.
  - `ddcore restore` verifies the archive, restores it into an isolated target, migrates, and
    times each phase. `--smoke` adds a check pass.
  - `ddcore maintenance on|off` pauses writes and jobs on every process and shows a Desk banner.
  - `DDCORE_DATA_DIR` overrides `dataDir`. `ops.backupMaxAgeHours` warns on a stale backup.

- S3-compatible file storage (PRD-05): `DDCORE_STORAGE=s3` with `DDCORE_S3_*` (AWS, R2, MinIO,
  B2) beside the default `local` backend under `<dataDir>/files`. `file_url` is the only name on
  both; a download is permission-checked by the server and then streamed, or redirected to a
  short-lived presigned URL; deleting a File or its document removes the bytes after commit, and
  a failed upload removes what it wrote. Mail, export and the CLI read through the store. See
  [storage](docs/agent/storage.md).
- Document sharing (SEC-03): the Core `Document Share` DocType grants one user `read`, `write` or
  `share` on one document, given by someone holding the `share` right and capped by their own
  write. A share stands in for a missing role grant on reads, lists, counts, link search, saves,
  `dbSet`, attachments, versions and comments, print, SSE and notifications, while controller
  hooks, workflow `allowEdit` and docstatus still apply. **Override security scope** lifts the
  recipient's User Permission scopes for the granted rights only. Renames move shares and
  deletions remove them; every grant, change and revocation is an audit event. `/api/shares/*`,
  `ddcore.share.*` and a Desk sidebar section. See [sharing](docs/agent/sharing.md).
- Global search (OPS-08): a Mod+K palette in the desk and `GET /api/search/global`, matching the
  title and search fields of every DocType the user can list, with roles, scopes, shares and field
  levels applied. `globalSearch` on a DocType opts it in or out. Core log DocTypes are opted out.
- Kanban and Gantt list views (OPS-08) through `defineListView({ kanban, gantt })`. Dragging a
  Kanban card saves its Select field.
- Single sign-on through OpenID Connect (SEC-05, partial): Google, PocketID or any OIDC provider,
  configured with `DDCORE_OIDC_*` in `.env`. It signs in existing Users only, linked by a verified
  e-mail address. `auth.passwordLogin: false` in `ddcore.json` leaves single sign-on as the only way
  in, except for Admin. See [authentication](docs/agent/auth.md).
- Core/app compatibility contract (PRD-07): `defineApp({ ddcore: "<range>" })` declares the ddcore
  releases an app supports, and a binary outside the range refuses to load it. `ddcore doctor` and
  the export manifest report each app's version and range.

### Breaking

- A binary older than the one that last ran `migrate` on a database now refuses to open it,
  naming the core or app version that is older. Roll forward, or pass `--allow-older-binary`
  (`DDCORE_ALLOW_OLDER_BINARY=1`) when the migrations since were expand-only. Databases migrated
  before this release have no ledger row and are not checked until their next `migrate`.
- `storage.Store` has a new `List` method; an embedder with its own store must implement it.

- An app whose `version` is not a version — `"1"`, `"1.0"` and `"1.0.0"` are all fine, `"beta"` and
  `"1.0.0-rc.1"` are not — no longer loads. Fix the value or remove it.

### Fixed

- `^` and `~` with an abbreviated version now bound what the author left out, as npm does: `^0` is
  every `0.x`, `^0.0` every `0.0.x`, and `~1` every `1.x`. The spelt-out forms are unchanged.
- A job whose write is refused by maintenance mode goes back to the queue without consuming an
  attempt and without an Error Log row, instead of failing — on its last attempt it used to die of
  a pause that was nobody's fault.
- Global search applies its five-hits-per-DocType cap after ranking, so an exact match is no longer
  lost behind more recently modified rows, and `%` and `_` in the search text now match themselves
  instead of acting as wildcards.
- A `migrate` run by a build with no release version (a `dev` binary) no longer replaces the
  ledger's core version, which silently disarmed the rollback guard for every later binary.
- `ddcore.share.*` refuses a right it does not know (`override_scope` for `overrideScope`) with the
  same `417` as the endpoint, rather than dropping it and granting a share without it.
- Single sign-on clears the client address's failed attempts on a successful sign-in, as a password
  sign-in does, and a failure on the server's own side is reported as `sso_error=server` rather than
  blamed on the provider.
- `--allow-older-binary=true` is accepted; only the bare flag used to be.
- `ddcore doctor` reports a rollback refusal as its own critical instead of "the apps could not be
  loaded", and prints an S3 bucket with no prefix without a trailing slash.
- `ddcore restore` warns when it stops after the database was replaced: the target keeps the
  restored database and stays paused.
- A list view listed in `views` but never configured no longer shows a button that falls back to
  the table.

## 0.14.0 — 2026-09-16

### Breaking

- A new site is English and US dollars. `ddcore init`, and a `ddcore.json` that omits `lang` or
  `currency`, now mean `en` and `USD` instead of `pt-BR` and `BRL`. A site that relied on the old
  default must declare `"lang": "pt-BR"` and `"currency": "BRL"` before upgrading, or its screens,
  its catalogue lookups and its money precision all change underneath it.

### Fixed

- The quickstart works end to end on a published binary: `ddcore init` wrote an auth block of
  zeros that every later command refused (`auth.sessionDays must be greater than zero`), and
  `ddcore new-app` swallowed the error and left the app unregistered. `init` now writes the same
  defaults `Load` starts from, `new-app` reports a registration failure instead of hiding it, and
  neither persists environment overrides or resolved absolute paths into the file.

## 0.13.0 — 2026-09-16

### Added

- The sign-in screen can carry a notice and offer a demo account whose button fills the form in,
  configured in `ddcore.json` (`login.notice`, `login.demoUser`, `login.demoPassword`) or per
  deployment with `DDCORE_LOGIN_NOTICE`, `DDCORE_LOGIN_DEMO_USER` and `DDCORE_LOGIN_DEMO_PASSWORD`,
  and served to visitors as `site.login` in `/api/boot`. The account is offered only when both
  halves are set.
- The documentation site moved to its own domain: the reference is served at `ddcore.dev` and the
  public demo at `demo.ddcore.dev`.

## 0.12.0 — 2026-09-16

### Added

- Field permissions (SEC-02): a `permlevel` on a field, and permission rows per level, remove what
  a user may not read from documents, lists, export, Version diffs, print, per-recipient
  notifications and webhook payloads, and refuse a filter, sort, grouping or aggregate over them.
  An unwritable change is refused before the hooks run; values the user never saw survive a save
  and are carried by an amendment; a child table is judged by its parent. `fieldLevels` reaches
  the Desk, and `ddcore.redact` is there for app code, which stays trusted otherwise. See
  [field permissions](docs/agent/field-permissions.md).
- `doc.applyWorkflow` applies a workflow transition from server code, with the same guards the
  HTTP endpoint uses.
- The documentation site: a landing page and the human guides, built with VitePress and published
  to GitHub Pages.
- The Desk steps date parts with the arrow keys, and spells date placeholders in the reader's own
  letters rather than in English ones.

### Fixed

- Access scopes (SEC-01) no longer have a way around them: `db.exists` and `dbSet` apply them — a
  `dbSet` on a child row resolves its scope under the parent — Dynamic Link fields are checked on
  direct reads and writes, a scoped user is refused `User Permission` itself, and webhook
  administration and delivery are closed to scoped users below `ignorePermissions`.
- Workflows close the same class of escape: insert, delete and `db.setValue` no longer bypass a
  workflow's guards, `ignorePermissions` does not lift them, and a workflow is validated when the
  app loads rather than at the first transition.
- The vault's master key no longer leaks through `ddcore.secret`, and renaming a record re-keys
  its secrets instead of orphaning them.
- Assignments: an update to a `ToDo` cannot rewrite the assignment or spoof `assigned_by`, the
  side effects are isolated in savepoints, and assigners and co-assignees see their own tasks.
- Print: the page format and orientation are written into the HTML, Currency prints in the site
  currency, the columns block renders, and a disabled `Letter Head` can neither become nor clear
  the default.
- A notification rule no longer offers internal DocTypes as targets, `Audit Event` refuses
  `dbSet`, a Single is never "new" even before its first save, and the workspace dashboard link
  lights only on the dashboard.

## 0.11.0 — 2026-09-15

### Added

- Declarative approval workflows (OPS-04): `defineWorkflow` declares states bound to `docstatus`,
  the fields each state lets a role edit (`allowEdit`), and transitions with an action, the roles
  allowed to take it, `allowSelfApproval` and a synchronous TypeScript condition. The transition
  engine locks the row, writes a `workflow.transition` audit event and a timeline comment, and
  refuses a state's frozen fields to everyone. The Desk draws the action buttons and the state
  pill and locks what the state locks, over `/api/.../workflow` and its actions endpoint. See
  [workflows](docs/agent/workflows.md).
- A dialog's field properties can be changed after it opens, and the Desk keeps the active tab of
  a form in a query parameter, so a reload and a shared link come back to the same tab.

## 0.10.0 — 2026-09-15

### Added

- User access scopes (SEC-01): the Core `User Permission` DocType gives a user `allow`/`for_value`
  rules, optionally limited to one DocType, enforced below the SDK on lists, counts, link search,
  direct reads, the whole document lifecycle, export, attachments, versions and comments, SSE
  events and notifications, `Dynamic Link` fields included. Two users with identical roles, System
  Manager among them, stay isolated; `ignorePermissions` skips role checks but not scopes;
  `Admin` and the framework's own elevated contexts are unscoped, and a scoped user is
  refused `User Permission` itself, so scope administration belongs to unscoped admins.
  Grants and revocations are recorded as `permission.scope_grant` and `scope_revoke`. See
  [scopes](docs/agent/scopes.md).

### Fixed

- A Link field offers fresh choices after it is cleared, instead of the list it had before.

## 0.9.0 — 2026-09-15

### Added

- Print templates and PDF (OPS-01): a standard layout built from the DocType's metadata, app
  templates in `print/<name>.print.ts` through `definePrintTemplate` (header, section, keyValues,
  table, totals, columns, headings, rules, page breaks and raw HTML), the Core `Letter Head`
  DocType with a single default, `print`, `pdf` and letterhead endpoints, and PDF through
  Gotenberg, a configured command or a local headless Chrome. Read permission and the caller's
  scopes apply to the document, and the Desk previews it at
  `/app/[workspace]/[doctype]/[name]/print`. See [print templates](docs/agent/print.md).
- Specialized list views: `defineListView` gains a Calendar and a Card view beside the table, with
  a switcher whose choice is kept in the URL and in the browser, so a link opens on the view it
  was shared from.

## 0.8.0 — 2026-09-14

### Added

- The workspace became part of the route: the Desk lives under `/app/[workspace]`, with a
  workspace selector and a sidebar filtered to that workspace's items, and internal navigation
  carries the prefix. An old `/app/<DocType>` link still opens the DocType and moves itself into
  the workspace that owns it.
- A mobile layout: a top bar, a drawer with a backdrop that dismisses itself on navigation, and
  the options menu opening on a tap of the avatar.
- The Version list gained DocType and Document filters and columns, so a record's history can be
  found without a query.

## 0.7.1 — 2026-09-14

### Added

- `defineListView({ modifiedColumn: false })` hides the Modified column for a list that has a more
  meaningful date of its own.

### Fixed

- A list fetches the type column behind a `Dynamic Link` column, so the link resolves instead of
  rendering a bare name.

## 0.7.0 — 2026-09-13

### Added

- Assignments and pending work (OPS-05): the Core `ToDo` DocType, `assign`, `complete` and
  `revoke` on a document, `GET /api/todo/pending` filtered by access to the referenced documents,
  a due-date reminder delivered through the notification inbox, an assignment widget on the form's
  sidebar and a `/app/todo` central with a badge in the sidebar. An assignment does not grant
  access: revoking access hides the task. See [assignments](docs/agent/assignments.md).
- Administrative audit coverage (PRD-06): one `Audit Event` ledger, which absorbs the Vault Audit
  Log, written on the caller's transaction for what was allowed and directly on the pool for what
  was denied, immutable against API and Desk mutations, with recursive redaction of sensitive
  payloads, a scheduled retention sweep through `ops.auditRetentionDays` and
  `ddcore audit list|purge`. It covers role grants and revocations, account status, invitations,
  unlocks and sessions, scope grants, workflow transitions allowed and denied, job cancel, retry
  and purge, vault reads and writes, webhook replay and method authorization denials. See
  [audit](docs/agent/audit.md).

## 0.6.0 — 2026-09-13

### Added

- Notifications (OPS-03): `defineNotification` declares event and date rules; an occurrence is
  written per recipient on the caller's transaction, deduplicated by the database, listed in an
  authorized Desk inbox with read and unread state, optionally e-mailed through a template, and
  pushed to the recipient alone over SSE after the commit. See
  [notifications](docs/agent/notifications.md).
- Outgoing webhooks (OPS-06): `Webhook` subscriptions for document lifecycle events and for app
  events through `ddcore.webhooks.emit`, a `Webhook Delivery` and its job written on the caller's
  transaction, Standard Webhooks signatures with a key held in the vault, a `webhook-id` stable
  across retries and replay, a per-webhook timeout and attempt count with exponential job backoff,
  System Manager replay recorded in the audit, `DDCORE_WEBHOOKS=off` for a rehearsal, retention,
  `ddcore webhooks` and a doctor section. Delivery is at least once, so a receiver deduplicates on
  `webhook-id`. See [webhooks](docs/agent/webhooks.md).

### Fixed

- A Go error keeps its type when it crosses a JS frame, so a controller sees a validation error as
  one instead of as an anonymous failure.

## 0.5.4 — 2026-09-13

### Added

- `defineListView` gains `fields` (fetch more than the columns, for an indicator, a badge or a
  formatter), `badges` (extra indicators drawn after the status, in the same cell) and
  `filterOptions` (choices appended after a divider to a standard Select filter, each applying its
  own filters in place of the equality).
- `defineListView({ docstatusFilter: false })` hides the docstatus filter of a submittable DocType
  that already shows a status field of its own.

## 0.5.3 — 2026-09-12

### Fixed

- Check filters and the clear button line up with the other inputs of the list's filter row.

## 0.5.2 — 2026-09-12

### Added

- In development, `DDCORE_MAIL_DEBUG` redirects every SMTP message to one address through plus-tag
  subaddressing (`tenant@example.com` becomes `admin+tenant_example_com@…`), with the real
  recipient kept in an `X-Original-To` header.

## 0.5.1 — 2026-09-12

### Added

- The `Vault` fieldtype (SEC-06): a per-record secret encrypted with AES-256-GCM in its own
  `ddcore_vault` schema, with no column on the document's table, read and written through
  `ddcore.vault.*`, redacted everywhere a value would otherwise travel, audited, editable in the
  Desk and reported by `ddcore doctor`. See [vault](docs/agent/vault.md).
- `ddcore.http` gains `put`, `patch` and `del`, and a response carries its headers with canonical
  HTTP casing. `opts.method` no longer overrides the verb the function name already chose.

## 0.5.0 — 2026-09-12

### Added

- The MCP server extracts and fills translation catalogues, through `i18n_extract` and
  `set_translations`, so a missing key can be found and answered without leaving the tool.

## 0.4.1 — 2026-09-12

### Changed

- No container image is published any more. An app builds its own image and downloads the binary
  of the release it pins.

### Fixed

- The sign-in screen focuses its first field without `autofocus`, so a password manager and the
  browser's own restore no longer fight over it.

## 0.4.0 — 2026-09-12

### Added

- A form keeps what was typed. Edits are kept in the browser, one draft per user and record, for
  seven days and written as they are made; leaving a dirty form asks first, inside the desk and
  through the browser's own dialog on a reload; coming back puts the draft back and says so, and
  asks when someone else saved the record in between. "Discard changes" in the form's menu, and
  `frm.discardChanges()`, return to the document as it was loaded without asking the server again.
- A form field declares its width — `sm`, `md`, `lg` or `full` — and a line is four slots, so
  narrow fields pair side by side and collapse to full width on a phone. Date, Month, Time, Int
  and Percent default to `sm`, Datetime, Float and Currency to `md`, the fallback is `lg`, and the
  types that are unreadable narrow keep `full`.

### Breaking

- The `Column Break` fieldtype is gone: a form's shape now comes from the width of its fields, and
  the same field no longer renders at a different size on either side of a break. A DocType that
  still declares one loads — an obsolete fieldtype is dropped when the app is read, with a warning
  naming the DocType — so an app written against an older ddcore keeps working while it is
  cleaned up.

### Fixed

- `Cmd+S` saves what is still being typed, instead of the value the field had before the caret
  entered it.
- The generated `tsconfig` no longer emits `baseUrl`, the core's own TypeScript is typechecked,
  and `hasPermission`'s declared document type is wide enough for what it is given.

## 0.3.0 — 2026-09-11

### Added

- Business email (OPS-02): file-based templates in `mail/<name>.mail.ts`, a block vocabulary
  rendered into both parts of the message, `ddcore.sendMail` writing on the caller's transaction,
  authorized `File` attachments with a size cap, and an `Email Delivery` record per message. The
  framework's own invitation and recovery messages are two of these templates, and a `sensitive`
  template keeps its arguments out of the delivery record. See [mail](docs/agent/mail.md).

## 0.2.0 — 2026-09-11

### Added

- Job administration (PRD-04): `ddcore jobs`, a System Manager-only HTTP surface and MCP tools
  inspect, retry and cancel jobs — a running job is cancelled cooperatively — with per-queue and
  per-method metrics, a daily retention sweep whose windows live in `ops`, and the `request_id` of
  the request that queued the work carried onto the job. See [operations](docs/agent/ops.md).
- An action can be attached to a field, not only to the document, so the button sits beside the
  input it fills and the framework can see it: a translated label, `hidden` and `dependsOn`
  instead of an `HTML` field, an `esc()` per app and a delegated click listener.
- `desk.logo` finally does something: the brand mark is the one declared by the first app in load
  order, or the initial of the site's name, in the sidebar and on the sign-in screens alike.

### Breaking

- `site` is gone from `ddcore.json`, from `config.File` and from `engine.Config`. The site's name
  is now the `title` of the first app that is not the core, resolved on the server so that the
  sidebar, a recovery e-mail and `/health` cannot disagree, and translated for whoever is reading.
  A config that still carries the key loads and ignores it, so no deployment breaks — but every
  one of them is renamed by its app on the next start, and a site that wants a name other than its
  app's title no longer has a way to say so. `doctor` loses its `site:` line, and `/health` can no
  longer tell two deployments of one app apart.

### Fixed

- The site's name is a catalogue key wherever it is shown, in the sidebar and in the mail.

## 0.1.2 — 2026-09-11

### Fixed

- An account is administered, and a refusal says so. `User` granted `All` an `ifOwner` read, and
  the owner of a User row is whoever created the account, so the grant freed no row at all while
  still passing every doc-less check: the DocType travelled in the boot payload, the desk listed
  it, and the list came back `200` with the query quietly narrowed to nothing, where a refusal
  belonged. `User` is now System Manager only; what a person may do to their own account keeps
  going through the profile service.
- The server writes its log to stdout, where a platform reads it as output instead of painting
  every line — INFO included — as an error. Every other command keeps the log on stderr, because
  there stdout belongs to the command: an exported NDJSON, a printed API key, `doctor --json` or
  the MCP JSON-RPC stream.

## 0.1.1 — 2026-09-11

### Changed

- The example app left this repository for
  [ddcore-demo](https://github.com/jrvidotti/ddcore-demo), where it is built against the published
  binary and takes the path a real app takes. What stays here is `apps/testapp`, the fixture
  `internal/acceptance` needs.

### Fixed

- The language picker's own labels and the profile's field rows are translated like everything
  else.

## 0.1.0 — 2026-09-11

The first release.

### Added

- The framework: file-based DocType metadata with controllers and lifecycle hooks, role
  permissions, REST and RPC endpoints, a generated Desk with forms, lists, reports and
  workspaces, jobs and a scheduler, files, Version and Comment, the `ddcore` CLI, an MCP server,
  and typings generated from the metadata.
- Internationalization: English is the canonical language and every user-facing string is a
  catalogue key, translated on the server for metadata and mirrored into the runtime for app code.
  `ddcore i18n extract` builds the catalogue and a missing key fails the build; the language
  resolution chain, one timezone per site and regional formatting come with it, and the Go core,
  the CLI, the MCP server and the developer errors all speak keys. See [i18n](docs/agent/i18n.md).
- Money and dates (DAT-06): an exact decimal rounding kernel shared by the server, the app runtime
  and the Desk, the site's currency precision and rounding rule applied to coercion and to what a
  Currency field commits, and one site clock. An impossible precision is refused at `migrate`.
- Export (DAT-02): a whole DocType with its children and an attachment manifest, over
  `GET /api/export/<DocType>` and `ddcore export`, permission-checked on the server, capped, and
  flagged as truncated only when something was actually cut. The list's export offers the whole
  filtered set rather than the page on screen. See [export](docs/agent/export.md).
- Migrations (DAT-04): declared renames through `renamedFrom`, a refused conversion where data
  could be lost, and phased patches in `patches/NNNN_name.ts`. Compound business keys through
  `uniqueKeys` (DAT-05), checked before the write and enforced by a partial unique index under
  concurrency. See [migrations](docs/agent/migrations.md).
- `extendDoctype` (DAT-09): one app adds fields and property overrides to another app's DocType,
  versioned like the rest of the metadata. See [extensions](docs/agent/extending.md).
- Single DocTypes through `isSingle` (DAT-03): one configuration document per DocType, with
  defaults before the first save, permissions on the read and on that save, and Desk editing at
  `/app/{doctype}`.
- Authentication and account self-service (SEC-04): password recovery and invitations over public
  routes with single-use tokens, a password minimum every path goes through, a lockout that
  answers `429` instead of taking another guess, one source for the session TTL and a cookie that
  says `Secure`, a profile with sessions and API keys of one's own behind an avatar menu, and
  `ddcore user invite|unlock|sessions` from the terminal. A minimal SMTP transport with a
  pluggable send method carries the messages. See [authentication](docs/agent/auth.md).
- Configuration split (SEC-04): the environment in `.env`, the site's decisions in `ddcore.json`,
  with an integration secret living in the environment rather than in a column (SEC-06).
- Operations (PRD-03): liveness separated from database readiness, a request correlation id and a
  duration on every request, queue failure and age signals, thresholds in `ops`, and a `doctor`
  that reports instead of dying when the database is down. See [operations](docs/agent/ops.md).
- Distribution (PRD-02): a release workflow that publishes static archives for Darwin and Linux on
  amd64 and arm64 with `SHA256SUMS`, an `install.sh`, `ddcore version` with the version injected
  at build time, and `PORT`/`DATABASE_URL` support for a platform deployment.
