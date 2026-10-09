# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Added

- **A grid shows that a script is loading its rows.** `frm.setDfProperty(table, "loading", true)`
  replaces "No rows" with a spinner and "Loading...", or dims the rows on screen during a reload.
  While it is on, the grid's field buttons, its actions on the selected rows and "Add row" are
  disabled. See "A grid a script fills" in `form-api` (#122).
- **Totals above a list.** `defineListView({ summary })` puts cards above the rows, styled like a
  report's summary: a count or the sum of a field (`aggregate: "count" | "sum:<field>"`) over
  every row the list's filters and search match, not only the page on screen, recomputed as the
  list reloads. A card may narrow itself with its own `filters`, and takes a `datatype` and an
  `indicator`. It runs as the reader, through the list API. See "Totals above the list" in
  `form-api`.

### Changed

- **`ddcore.call` runs the progress bar at the top of the desk** while it is out, as `frm.call`
  already did. A page filled by a slow whitelisted call no longer looks idle (#122).
- **A field button waits for its `onClick`.** When `onClick` returns a promise, the button stays
  disabled, with a spinner in place of its icon, until the promise settles. Its error is shown.
  Clicking "Reload" again while it runs does nothing (#122).

## 0.27.21 — 2026-10-08

### Added

- **An app adds its own cards to the profile page.** `defineProfileSection` in
  `@ddcore/desk-sdk`, called from a `desk.include` or `portal.include` script, puts a card after
  the Password card on `/app/profile` and `/portal/profile`: a `title`, read-only `info` lines
  from what `load` returns (`null` hides the card), `fields` rendered as in a form, and a
  `submit` whose errors show on the card and whose success clears the fields and reloads it.
  Several apps may each add theirs. See "`defineProfileSection`" in `form-api` (#121).

### Changed

- **The profile page shows the Password card only to someone with a local password.**
  `profile.getMyProfile` answers `hasPassword`. Someone who signs in only through a credential
  provider or SSO has no current password to give `changeMyPassword`, so the card was a dead end
  for them (#121).

### Fixed

- **A desk user whose roles open none of the space's workspaces is told so.** The desk home said
  "No workspace declared" and suggested `ddcore new-app`, as on a site with no workspaces. Boot
  now sends `workspacesDenied`, and the home names the account and the tenant, asks for a role
  from an administrator and offers Sign out. See the workspace `roles` in `report-api` (#120).

## 0.27.20 — 2026-10-08

### Added

- **A form script reads a grid's selection and acts on it.** On a Table or Report field with
  `gridSelect: true`, `frm.getSelectedRows(field)` returns the ticked rows still on screen
  (after `gridSearch` and `gridFilters`), in screen order, and `frm.clearSelection(field)`
  unticks them. `frm.addGridAction(field, { label, onClick, condition, primary, key })` puts a
  `label (n)` button in the grid's toolbar, as a list's `actions` do: `onClick(rows)` gets the
  selected rows that pass `condition`, and the selection is cleared once it settles.
  `removeGridAction(field, key?)` takes actions away, and `clearButtons()` clears them at each
  `refresh`. A tool Single no longer needs a `selected` Check column to act on chosen rows. See
  "Actions on a grid's selected rows" in `form-api` (#118).
- **A Select draws its indicator, with an icon per value, in grids, lists and reports.** A
  Select that declares `optionColors` or the new `optionIcons` (`{ Enabled: "check" }`, keyed by
  the canonical value, names from the desk's icon set) shows each value as its coloured
  indicator — the icon in place of the dot, then the translated label — in a form grid's
  read-only cells, list columns, report columns and read-only fields, the row dialog included.
  `optionIconOnly: true` draws the icon alone in table cells, with the label as the tooltip, for
  a narrow column. An unknown `optionIcons` name fails the load. A report column takes the same
  three properties. A Select without them still reads as text (#119).

### Fixed

- **The list's and a report's status cells show a Select's translated label.** They used the
  catalogue (`__(value)`) and ignored `optionLabels`, so a self-describing Select showed its
  canonical value there while the form showed its label (#119).

## 0.27.19 — 2026-10-08

### Changed

- **Credential sign-in asks for the organization instead of listing tenants.** The provider's
  tab on `/login` has an organization input (the tenant id) and goes to
  `/login/<provider>/<tenant>`, which shows that organization's name and asks for the username
  and password. `GET /api/auth/credentials/<id>/tenants/<tenant>` answers one tenant
  (`{data: {id, title}, tenancy}`) or a 404 for a missing, disabled or opted-out one, and each
  miss counts against the client address. `GET /api/auth/credentials/<id>/tenants` no longer
  lists anything: it answers `{data: [], tenancy}`. `/api/boot` adds `site.login.tenancy`.
  Before this, the sign-in screen showed every tenant that offered the provider to anyone who
  opened it. See "Sign-in through an app" in `auth` (#117).

## 0.27.18 — 2026-10-08

### Added

- **Sign-in through an app: credential providers.** An app declares `auth.providers` in
  `defineApp`: a `label`, the `userField` of User (a Data field it adds with `extendDoctype`)
  that links an account to a username of another system, an optional `enabled()` per tenant,
  a synchronous `verify({ username, password })` and an optional `afterSignIn`. The sign-in
  screen gets a tab per provider where the person picks a tenant (from a public list of the
  tenants whose `enabled()` says so), types that system's username and password, and gets the
  same session a password sign-in gives. ddcore throttles before calling `verify` (per username
  in that tenant, and per address with the password sign-in), signs in only a User an
  administrator linked beforehand, treats a `verify` that throws as "unavailable" without
  counting it against the account, runs the network call outside the session's transaction,
  and audits `account.login_credentials`. `POST /api/auth/credentials/<id>/login`,
  `GET /api/auth/credentials/<id>/tenants`, and `site.login.credentials` in `/api/boot`.
  Unlocking a user clears the provider's counter too. Before this, the only ways in were a
  local password and OpenID Connect. See "Sign-in through an app" in `auth` (#115).
- **Placeholder addresses for Users without a mailbox.** A recipient under `.invalid` is
  dropped from `sendMail`; with nobody left, nothing is queued and the answer is
  `{ delivery: "", skipped: true }` instead of an error. Forgot-password sends nothing to such
  an address and inviting one is refused. Before this, mail to it was queued and failed in the
  worker (#115).

## 0.27.17 — 2026-10-07

### Added

- **Tool Singles: a form the user fills in and never saves.** `tool: true` on a Single makes it
  a screen whose script hands what is on it to a service, such as "type a CPF, tick the
  installments, get a link". Anyone who can read it edits its level-0 fields and grid rows; the
  desk shows no Save, never marks it "Not saved", keeps no draft, does not ask before leaving and
  hides the sidebar; the server refuses every write to it (`save`, `insert`, `dbSet`, REST, MCP),
  to Admin too. `tool` on a DocType that is not a Single fails validation. Before this, a Single
  was read-only to a user without `write`, and with `write` it carried Save and a stored row that
  every user shared. See "DocType properties" in `fieldtypes` and "Tool Singles" in `form-api`
  (#114).

## 0.27.16 — 2026-10-07

### Added

- **An API key can carry a prefix in its id.** `ddcore.users.createApiKey(user, { prefix })` and
  `ddcore apikey <user> --prefix p` issue a key whose id is `<prefix>.<random>`, e.g.
  `acme.ji26rchc5g:<secret>`, so whoever holds it can tell which tenant or integration it
  belongs to without asking the database. The prefix has a tenant id's shape; anything else is a
  `ValidationError`. Keys issued without one keep their random id, and every existing key keeps
  working. (#113)

## 0.27.15 — 2026-10-07

### Fixed

- **The sidebar's workspace switcher lists workspaces in alphabetical order.** Boot sent them in
  Go map order, so the list, and the workspace the desk opens when none is remembered, changed
  from one load to the next. They are now sorted by the label the desk shows (the name when
  there is none), collated for the session's language and ignoring case.

## 0.27.14 — 2026-10-07

### Fixed

- **`ddcore maintenance on|off` works once tenancy is applied.** Switching the flag first ran
  `CREATE TABLE IF NOT EXISTS ddcore_maintenance` on the confined pool. The tenant role may write
  that table but not create anything in `public`, so the command failed with "permission denied
  for schema public". The tenancy cutover could not leave maintenance with its last step, and
  the `maintenance_set` MCP tool failed the same way. The ops tables are now created on the
  system pool. The same fix lets `ddcore backup` record its run in `ddcore_backup_log` again, so
  `doctor` sees backups taken after tenancy. (#112)

## 0.27.13 — 2026-10-07

### Fixed

- **`ddcore migrate` no longer hangs when one run alters `tab_webhook` and writes a document.**
  The migration that turns tenancy on adds `tenant` to `tab_webhook`. If the same run created a
  new app role, a fixture, or a document from a patch or hook, the write looked up the webhook
  subscribers on another connection. That connection waited for the migration's lock, and the
  migration waited for it, with no timeout. `ddcore start` never became ready. Inside a
  migration the subscribers are now read on the migration's own transaction. A subscription
  committed before the migration still receives the documents it writes. (#111)

## 0.27.12 — 2026-10-07

### Fixed

- **Inside a tenant, `getRoles` sees a User the same transaction inserted.** The role lookup
  placed a user it could not find on another connection in the platform space. So
  `ddcore.getRoles(user)` answered `["All"]` for a User just created in a tenant, inside
  `ddcore.test.asUser` too, and that answer could stay cached for the user. It now looks in the
  transaction first. (#110)
- **Inside a tenant, a `Link` to `User` accepts the operator who entered it.** `Admin` and the
  platform's System Managers have no `User` row in a tenant, so recording
  `ddcore.session.user` failed with "Invalid link", in the scratch tenant of `ddcore test` and
  in the desk alike. A `Link` or `Dynamic Link` to `User` now accepts an operator there, and its
  title is the operator's full name. See `tenancy`. (#110)

## 0.27.11 — 2026-10-07

### Added

- **Shared DocTypes only server code reaches.** On a site with tenancy, a shared DocType that
  declares `tenantAccess: "server"` is refused to every client inside a tenant, whatever the
  role. That covers boot, the desk, REST, `/api/meta`, search, link titles, export, reports,
  uploads and a tenant's webhooks; workspace entries that open it show in the platform space
  only. A tenant's server code, meaning a call that ignores permissions (`db.getAll`,
  `db.getValue`, `db.exists`, `getDoc(…, { ignorePermissions: true })`, jobs), reads the
  platform's rows. Its `insert` and `save` with `ignorePermissions: true` write them in the
  platform space on the same transaction. Its series, Version, audit events, vault secrets,
  webhooks, notifications and realtime events land there, and the Version and audit events are
  stamped with `source_tenant`. `delete`, `setValue` and rename stay refused in a tenant. This
  is for platform data every tenant's code contributes to, such as a paid lookup cache. See
  `tenancy` (#108).

### Fixed

- **A DocType made `shared` after tenancy was applied is shared in the database too.**
  `ddcore migrate` used to report nothing to do and leave the table with row-level security,
  the tenant policy, the `tenant` column and the `(tenant, id)` key, so inside a tenant the
  DocType read as empty and a `Link` to it failed. It now drops the policy, turns row-level
  security off, keys the table by `id`, rebuilds the indexes without the tenant and drops the
  column, for the DocType and its child tables. It refuses, naming the tenants and their row
  counts, while any row belongs to a tenant; a `beforeSchema` patch is where those are moved
  or deleted. (#109)

## 0.27.10 — 2026-10-07

### Added

- **A scratch tenant for `ddcore test`.** On a site with tenancy, the tests of an app with
  `defineApp({ tests: { space: "tenant" } })` run inside a tenant the run creates in its
  rolled-back transaction. That is the default for an app with `space: "tenant"`, so an app
  whose tests use another app's tenant-only DocTypes adds the line. The tenant has every app's
  `onTenantCreate` applied, `Admin` entered and no real tenant's documents, and nothing of it
  survives the run. `ddcore.test.inPlatform(fn)` runs part of such a test in the platform space,
  for a shared DocType, a `Site Tenant` or a scheduled fan-out. Other apps' tests run in the
  platform space as before. See `tenancy` (#107).

## 0.27.9 — 2026-10-07

### Added

- **Tenant-only DocTypes.** On a site with tenancy, `space: "tenant"` on `defineDoctype`, or on
  `defineApp` as the default of every DocType of the app (`space: "any"` opts one out), keeps a
  DocType out of the platform space. Boot leaves it out, so the desk has no menu entry, search
  row or Link for it. Every read and write there is refused with "… lives inside a tenant: enter
  one (on the command line, pass --tenant)", whether it comes from the desk, REST, MCP, `ddcore
  eval` or `exec` without `--tenant`, a job or a hook. `ddcore import run` without `--tenant`
  leaves it out and says so. Code inside a tenant, `ddcore.tenant.run` and migration patches are
  unaffected. Before, such a DocType opened and saved in the platform space, and the row landed
  where no tenant sees it. Fixtures of a tenant-only DocType fail the load: use
  `onTenantCreate`. See `tenancy` (#105).

### Changed

- Boot leaves out the DocTypes the session's space may not read, and the reports on them. Inside
  a tenant that is `Site Tenant`, which was listed and then refused. A workspace entry with no
  `space` that opens a tenant-only DocType, or a report on one, shows inside a tenant only, and
  so does a grouped `links` item. An explicit `space` still wins (#105).

### Fixed

- `ddcore i18n extract` no longer reports an app's override of a core key as an orphan, and
  `--prune` (or `i18n_extract {prune: true}`) no longer deletes it. A key the app's code does not
  have but an app loaded earlier translates — the core included, in any language — is an
  override: it is written with `# overrides core` (or that app's name), listed as `override` by
  the CLI (the summary line gains an `N override` count) and under the new `overrides` list of
  `i18n_extract`, and kept by `--prune`. `set_translations` now accepts such a key instead of
  refusing it as not in the code (#106).

## 0.27.8 — 2026-10-07

### Fixed

- **Security:** a client can no longer set `doc.flags` by sending a `flags` key in its body. A
  REST create or update, the desk's save, submit and cancel, and MCP `insert_doc` and
  `update_doc` copied the key into the document, and the hooks took it as the write's flags, so
  a user with write permission got past any `validate` rule keyed on a flag the app's server
  code sets. The key is now dropped on every write, and a hook's `doc.flags` comes from the
  write alone: `{}` for one a client started, as documented. A `__before` key, which could stand
  in for `doc.getDocBeforeSave()` on an insert, is dropped the same way. Nothing changes for an
  app that sets `doc.flags` in server code (#104).

## 0.27.7 — 2026-10-06

### Added

- `ddcore exec --args -` reads the JSON arguments from stdin, and `--args-file <path>` from a
  file (`-` is stdin too). A payload carrying personal data no longer has to sit in argv, where
  `ps` and `/proc/<pid>/cmdline` show it to every user of the host, and a large batch is no
  longer capped by `ARG_MAX`. Malformed arguments are now reported before the database is
  opened (#103).

### Fixed

- `ddcore migrate --prune` plans its drops after the `afterSchema` patches instead of before
  them. A release that backfills and contracts in one go — both patches `afterSchema`, the
  contract dropping the old column itself — no longer fails, either on a refusal for data the
  backfill was about to copy or on a `DROP COLUMN` for a column the patch had already dropped.
  An orphan a patch writes into is now refused rather than dropped with what the patch wrote.
  `migrations` documents the contract-in-the-same-release pattern and why the contraction
  is not `beforeSchema` (#102).

## 0.27.6 — 2026-10-06

### Changed

- `limit=0` on `GET /api/resource/<DocType>` — and so `ddcore.db.getList(doctype, { limit: 0 })`
  in a desk script — returns every matching row, as `limit: 0` does on the server; it used to be
  read as "left out" and return a page of 20, so a bulk action over "all" silently acted on the
  first page. A negative or non-numeric `limit` is now a `ValidationError` instead of returning
  everything or 20. `form-api` documents the desk `getList`'s default of 20 (#101).

## 0.27.5 — 2026-10-06

### Added

- `ddcore.session` (`{ user, fullName, roles, lang }`) and `ddcore.hasRole(role)` in desk
  scripts, read from the desk's boot, so a form or list script can show a button only to the
  roles the server will accept. They hide UI and grant nothing: the method still checks (#93).
- `refDoctype` on a workspace number card or chart with a `method()`: the endpoint answers 403
  to a user without read on that DocType before running the method. A `method()` runs as the
  caller with no DocType check of its own, and `report-api` now says so, along with what the
  workspace `roles` and a `doctype` card enforce (#94).
- `defineListView({ toolbarActions })`: buttons on the list toolbar that run with no rows
  selected, in every view. `onClick(list)` gets `doctype`, the view's `filters` and `refresh()`;
  an optional `condition()` decides whether the button shows (#96).

## 0.27.4 — 2026-10-06

### Added

- `ddcore.errorLog.record(error, { method?, context? })` writes an `Error Log` row on a
  transaction of its own and returns its id, so code that catches a failure can leave it where
  operators and `ddcore doctor` look and go on: the row stays whether the caller's work commits or
  rolls back. It carries the request's id, or `job:<id>` in a job's body, lands in the current
  tenant, and never throws. The pattern for a job of independent steps — each in a savepoint, a
  failing one recorded rather than rethrown — is under "Savepoints" in `controller-api` (#92).
- `ddcore.job.current()` returns the running job's `JobInfo` in its body and callbacks (`null`
  outside a job), and `JobInfo` has `starts`: how many times a worker began the job, a run given
  back by a shutdown included. `attempt` stays the same across a give-back, so `starts > 1` is how
  a body doing non-idempotent work learns that an earlier run may already have done it. The admin
  job listing, `GET /api/jobs` and `ddcore jobs show` show `starts` too. The ops docs now say that
  `maxAttempts: 1` is not "runs at most once" (#91).
- `shutdownGraceSeconds` in `ddcore.json`, or `DDCORE_SHUTDOWN_GRACE_SECONDS` (default 30): how
  long a process told to stop lets its running jobs finish before it gives them back to the
  queue. Set the platform's stop timeout above it (Compose `stop_grace_period`, Kubernetes
  `terminationGracePeriodSeconds`) (#99).
- `ddcore tenant adopt <slug> --dry-run` prints, table by table, the rows the adopt would move
  into the tenant and every collision with rows the tenant already has, on the primary key and
  on each unique index (up to ten keys each), moves nothing, and exits non-zero on a collision.
  An app no longer has to re-derive the adopt's rule in SQL to fail before the cutover (#100).
- `ddcore import run|status|reconcile <dir> --tenant <slug>` loads an export into a tenant: the
  documents, the import ledger, the numbering series and the `import.run` audit events land in
  the tenant, and nothing else on the site moves. `--tenant` before `import` means the same.
  The load leaves out shared DocTypes and the `Admin` and `Guest` accounts, and refuses an
  attachment whose url a file of another space holds. The same export can be loaded into two
  tenants. A run records its tenant: `--resume` refuses another one, and `status` lists the runs
  of one space. The MCP `import` tool takes `tenant` (#100).
- `ddcore.db.tryLock(key)`: the lock of `ddcore.db.lock` without the wait. It returns `true` with
  the key held to the end of the transaction, or `false` at once while another transaction holds
  it, so a job can queue itself again and free its worker instead of parking it for as long as
  the holder takes. Scoped to the tenant, as `lock` is (#98).
- `ddcore.enqueue(..., { runAfterSeconds })`: the job's delay from now, in seconds (#98).
- Worker pools per queue: `"workers": { "default": 2, "bot": 2 }` in `ddcore.json` gives a queue
  workers of its own. A named pool serves its queue only; `default`, which the object must name,
  serves the default queue and every queue not named, the scheduler's included. A number still
  means that many workers for every queue. `ddcore jobs work --queue bot[,other] [--workers N]`
  starts only those pools, so a queue can have a process of its own; `DDCORE_WORKERS` overrides
  the setting with a number; `ddcore doctor` prints each pool (`workerPools` in `--json`) (#98).
- `ddcore.throw(msg, opts)` takes `status`, an HTTP error status (400-599) that overrides the
  type's, and `retryAfter`, seconds stored as `extra.retryAfter` that also set the `Retry-After`
  header. The SDK types `extra` (it reaches the JSON error body as `error.extra`) and exports
  `ErrorType`, the known types, and `ThrowOpts`. `controller-api` has the table of types and
  statuses, and the webhook examples refuse with `{ type: "PermissionError" }` (403) instead of
  a bare `ddcore.throw`, which is a 417 (#89).

### Changed

- A job's `timeout`, an administrative cancel and a worker shutting down now also cut the outbound
  call the job is waiting on — `ddcore.http.*`, `ddcore.files.save({ fromUrl })`,
  `ddcore.push.send`, an external database query, mail and webhook delivery — instead of
  waiting for it to return. A job's `timeout` is now a bound on its HTTP calls too. Calls made
  while serving a request are unchanged (#99).
- `ddcore tenant adopt` refuses with the list of colliding keys — table, columns and values —
  instead of stopping on the first raw primary-key violation. The docs now say what else an adopt
  moves: the platform space's Singles, vault secrets, Error Log, Audit Event, Version and
  Feedback rows (#100).
- `ddcore import` no longer refuses `--tenant` before the command. The import ledger
  (`ddcore_import_record`) is per tenant on a site with tenancy, and `ddcore tenant adopt` moves
  the platform space's ledger with everything else, so an export loaded before the adopt is not
  loaded again into the tenant after it. `migrate` re-keys the ledger; nothing to do (#100).
- `ddcore.enqueue` refuses a `runAfter` it cannot read with a `ValidationError`, and one given
  together with `runAfterSeconds`. It used to run such a job at once, which turned a job meant to
  wait into a hot loop. An ISO timestamp and `YYYY-MM-DD HH:MM:SS` are read as before, and a
  date (`YYYY-MM-DD`) or a timestamp without a zone is now read in the site's timezone too (#98).

### Fixed

- SIGTERM (or Ctrl-C) no longer drops the jobs a process is running. `ddcore start`, `ddcore dev`
  and `ddcore jobs work` stop claiming jobs, let the running ones finish within
  `shutdownGraceSeconds`, give the rest back to the queue with their attempt returned and no
  `onFailure`, and only then exit. They used to exit at once: a job blocked in `ddcore.http` stayed
  `running` until its lease expired and was then failed, its attempt spent, as the docs said it
  would not be (#99).
- An error thrown from app code answers its type's status for every type the server knows:
  `TooManyRequestsError` (429), `UnavailableError` and `MaintenanceError` (503) and
  `MethodNotAllowedError` (405) used to answer 500 from `ddcore.throw`. A Go error crossing app
  code — the `MaintenanceError` of a write made while the site is paused, say — keeps its status
  and its `Retry-After` instead of becoming a 500 with no header. An unknown type is still a 500
  (#89).
- A missing mail template, print template or notification rule raises `DoesNotExistError` (404)
  instead of the unknown `NotFoundError`, which answered 500 (#89).

## 0.27.3 — 2026-10-06

### Added

- `ddcore.http` takes `maxRedirects` (default 10): `0` returns the 3xx with its `Location`, so an
  app can follow a redirect itself and choose the headers it sends (#97).

### Fixed

- `Date.parse` and `new Date` in server code read a date-time written with a space instead of the
  `T`, as Postgres writes it: `"2026-10-05 10:00:00-04:00"` lost its time of day and
  `"2026-10-05 22:31:52.767353+00"` gave `NaN`. Both now give the instant Node gives (#90).
- `ddcore.log.*` and `console.*` no longer log an object as `"[object Object]"`: a plain object's
  keys become fields of the structured record (`ddcore.log.warn("retry", { status })` logs
  `msg="retry" status=…`), and other values join the message as JSON. A value with a cycle no
  longer makes `console.log` throw. `ddcore eval` prints the fields after the message (#88).
- `ddcore.http` and `ddcore.files.save({ fromUrl })` no longer send the caller's headers to
  another host on a redirect. A hop to another host, or from https to http, now drops every header
  the call passed except `Content-Type`; net/http only dropped `Authorization` and `Cookie`, so a
  token sent as `api_access_token` or `X-Api-Key` reached the redirect's target (#97).
- The access log, the panic log and the Error Log title no longer print what follows a method's
  name in `/api/method/<path>/<tail>`: they show `/api/method/<path>/…`. A webhook that carries a
  token in its `pathTail` wrote it to the logs on every delivery (#95).
- `ddcore <command> -h` and `--help` print the command's options (`ddcore test -h` lists
  `--filter`, `--app` and `-v`) or, for a command with subcommands such as `import`, `jobs` or
  `user`, its usage. Both failed with "unknown flag" and pointed at themselves (#87).

## 0.27.2 — 2026-10-05

### Added

- `defineReport({ description })` draws a line under the report's title, translated like `label`:
  a place to say what the report shows and how to read it (#81).
- `ddcore.http` sends binary and multipart bodies with `bodyEncoding`: `"base64"` sends a base64
  string `body` as raw bytes, and `"multipart"` sends an array of parts (`{ name, value }` or
  `{ name, base64, filename?, contentType? }`) as `multipart/form-data`, boundary included. A
  `Content-Type` header is now matched without regard to case. The type `HttpMultipartPart` is
  exported (#83).
- `ddcore.webhooks.verify(secret, headers, rawBody, { toleranceSeconds = 300 })` checks a
  Standard Webhooks request on an inbound method in one call: it decodes a `whsec_<base64>` secret,
  accepts several `v1,…` signatures, in one header or repeated ones, compares in constant time,
  enforces the timestamp window (`toleranceSeconds: Infinity` switches it off), reads header names
  in any case and returns `false` rather than throwing when a header or the secret is missing.
  `ddcore.crypto.hmacSha256` takes `{ keyEncoding: "utf8" | "base64" | "hex", output: "hex" | "base64" |
  "base64url" }`; without options it answers as before (#84).
- `ddcore.crypto.randomToken(bytes = 32)` (base64url from `crypto/rand`), `randomInt(min, max)` (uniform
  in `[min, max)`) and `sha256(data, { output })` (hex by default), for session tokens, one-time codes and
  the hash to store instead of a token (#85).
- `ddcore.push.send(subscription, payload, { ttl, urgency, topic })` delivers a Web Push
  message from server code: the payload encrypted for the browser's subscription (RFC 8291,
  `aes128gcm`, at most 3993 bytes) and the request signed with the site's VAPID key (RFC 8292),
  read from `DDCORE_SECRET_VAPID_PUBLIC_KEY`, `_PRIVATE_KEY` and `_SUBJECT`. It returns
  `{ status, body, headers }` whatever the status, so the app deletes a subscription that answered
  404 or 410. An endpoint must be `https:` outside development. `ddcore.push.publicKey()` gives
  the page the key it subscribes with, or `null`, and `ddcore push keys` prints a new pair, refusing
  a `--subject` that is not a `mailto:` or `https:` URL. See `push` (#82).

### Fixed

- `ddcore.utils.randomString` used `Math.random`, which is not safe for a credential. It now draws
  from `crypto/rand` without modulo bias, with the same `a-z0-9` alphabet and the same handling of
  `n`: a fraction rounds up and anything not positive gives `""`. It now throws above 65536
  characters. The ids ddcore generates for new documents share that bias fix (#85).

## 0.27.1 — 2026-10-05

### Added

- The desk draws `key` and `bug`.
- The desk draws 201 more lucide icons, 300 in all: money and commerce (`coins`, `piggy-bank`,
  `hand-coins`, `shopping-cart`, `store`, `package`, `warehouse`, `truck`, `barcode`, ...),
  people (`user-pen`, `user-check`, `id-card`, `graduation-cap`, ...), messages (`phone`,
  `message-circle`, `messages-square`, `video`, `megaphone`, ...), documents (`files`,
  `clipboard-list`, `file-spreadsheet`, `signature`, ...), time, places (`map`, `globe`,
  `church`, `hospital`, ...), status, actions, systems (`network`, `database`, `server`, ...)
  and more. lucide's old names `check-circle`, `x-circle`, `alert-circle`, `help-circle`,
  `circle-help`, `unlock`, `pie-chart`, `line-chart`, `smile` and `fingerprint` work too. The
  full list is under "Icons" in `report-api` (#80).
- `allowCreate: false` on a DocType says only server code creates its documents, such as a
  Transfer made by a button on another form. The desk then offers no way to make one, to
  **Admin** too: `/api/meta` reports `create` and `amend` as false, so the list has no **New**,
  a Link no `+`, the form no Duplicate or Amend, and Data Import no insert mode. A typed `/new`
  shows a notice. `POST /api/resource/<doctype>` and a spreadsheet insert are refused. Server
  code inserts as before. An extension may set it on another app's DocType. See "DocType
  properties" in `fieldtypes` (#79).
- The list shows the DocType's `description` under its title, translated like `label`: a place
  to say where the documents come from (#79).

### Fixed

- API Key, Error Log and Version show an icon in the desk's System menu (`key`, `bug`,
  `history`) instead of the dot an item without one gets.
- `/new` for a DocType the reader cannot create shows a notice with a link back to the list.
  Before, it opened a form that took the typing and whose save the server refused (#79).

## 0.27.0 — 2026-10-05

### Breaking

- An `icon` the desk does not draw now fails the load, on a DocType, a workspace or anything a
  workspace lists (`sidebar`, `shortcuts`, `links`), naming where it is:
  `workspace "Payments", sidebar[3]: icon "piggy-bank" is not one the desk draws`. Before, the
  desk drew a circle and nothing said so. **Upgrade:** run `validate_meta` (or start the
  server) and replace each name it reports with one from the list under "Icons" in
  `report-api` (#78).

### Added

- The desk draws `wallet`, `credit-card`, `banknote`, `landmark`, `percent`, `qr-code`,
  `send`, `inbox`, `undo-2`, `book-open`, `briefcase`, `building`, `wrench`, `file-text`,
  `folder-tree`, `chart-bar`, `pen` and `user-plus`, and accepts lucide's other names for icons
  it has: `triangle-alert`, `chart-column`, `ellipsis`, `square-check-big`, `edit-2`. The list
  in `report-api` is generated from the desk's own table, so it names every icon there is (#78).
- **Feedback.** Every desk user can report a bug, an improvement or a feature request from
  **Feedback** in the user menu. The dialog asks for the fields of the chosen type, takes up to
  10 files (picked, dropped or pasted screenshots) and an audio note recorded in the browser, and
  sends the current page's address and context data (both checked by default; the context is
  previewed before sending). It is written as a core `Feedback` document, in the platform space
  on a site with tenancy, stamped with its `source_tenant`. The platform's System Managers get it in the
  desk inbox, and the addresses in `"feedback": {"to": [...]}` (or `DDCORE_FEEDBACK_TO`) get a mail
  with the files. The author follows status and response under "My feedback". It is on by
  default; `"feedback": {"enabled": false}` or `DDCORE_FEEDBACK=0` turns it off. App scripts can
  open it with `ddcore.ui.openFeedback({ type, title })`. Endpoints: `POST /api/feedback`,
  `GET /api/feedback/mine`. See `feedback`.
- Audio files (`.webm`, `.ogg`, `.m4a`, `.mp3`, `.wav`, …) are served in place with their audio
  type, like images and PDFs, so an `<audio>` element plays them.
- The desk draws `bug`, `lightbulb`, `mic`, `square` and `trending-up`.

### Changed

- The sidebar's Notifications and To-Do links moved from the top of the menu to its footer,
  just above the user's avatar, so they no longer read as one of the workspace's modules.
  Their counters and the collapsed rail work as before.

### Fixed

- The desk's own icons that drew a circle — the open workspace menu's chevron, the assign
  dialog's and the editable title's — draw what they name; in the browser, an unknown name
  draws a circle with a console warning (#78).

<!-- #endregion releases -->
