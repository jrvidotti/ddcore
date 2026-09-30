# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Added

- **`setOnlyOnce: true`** on a field (#43): once the document exists, the value cannot change —
  the server refuses it on every write path (`save()`, REST, `ddcore.db.setValue` / `doc.dbSet`,
  Data Import), whatever the permissions, and the desk shows the field read-only. An empty value
  may be filled once; on a child row it holds for rows already saved. An extension may impose it
  on another app's field. Replaces a `hasValueChanged` check repeated in each `validate`, which
  `dbSet` skipped.

### Fixed

- **A scope, share, role or session changed in one process now reaches the others** (#45). The
  caches behind them live in each process, so a `User Permission` revoked by `ddcore eval
  --commit`, `ddcore exec`, an import or another replica kept its grant alive in the running
  server until a restart — a leak between tenants isolated by scope. Invalidations are now
  announced with Postgres `NOTIFY` when the transaction commits (never on a rollback), and the
  server, `ddcore jobs work` and `ddcore mcp` listen and drop the same keys; a listener that
  reconnects drops its whole cache, and `ddcore migrate` clears every process's cache.
  `ddcore.cache.del` from app code is announced the same way. Nothing to change in an app; a
  replica needs no restart after a revocation.

## 0.21.16 — 2026-09-30

### Added

- **`gridSearch: ["field", ...]`** on a `Table` or a `Report` field (#46): a search box above the
  grid that shows only the rows where one of those columns contains the text typed — case- and
  accent-insensitive, each word matched in any of the columns, a Link by its id and its title, a
  Select by its value and its label. A Table's fields may be `hidden` (checked at load). It
  combines with the active `gridFilters`, and selection and export follow the rows on screen;
  like them, it is display only and leaves the rows and their `idx` as saved.

## 0.21.15 — 2026-09-30

### Added

- `defineListView` takes `plainLinks: ["field"]`: those Link (or Dynamic Link) columns show the
  linked document's title as text instead of a link, so a click on the cell opens the row's own
  document. For lists whose rows are easily mistaken for what they link to (#42).

## 0.21.14 — 2026-09-30

### Fixed

- **`hideLabel` is accepted on a `Report` field** (#41), as on a `Table`: a tab holding only a
  Report shows just the grid, without repeating the tab's name. The meta was refused at load
  with "hideLabel is for a data field, not a Report".
- **Chrome no longer offers to translate a Portuguese desk "from English".** The desk shell shipped
  `<html lang="en">` and corrected it only after `/api/boot`; the server now stamps the resolved
  language (user, `Accept-Language`, site default) into the first response, and `loadBoot()` keeps
  the attribute in step on every boot, including the reload after an SSE event.

## 0.21.13 — 2026-09-30

### Added

- **`raw: { contentType }` on a whitelisted method** (#37) writes the returned string as the whole
  response body, with that content type and no `{data, messages}` envelope: what a provider that
  verifies a callback URL by reading a challenge back (Meta's `hub.challenge`) expects.
- **Raw body and headers for whitelisted methods** (#38). `ctx.request` gains `rawBody` (the exact
  bytes received) and `headers` (lower-cased names, without `cookie` and `authorization`), and
  `ddcore.crypto.hmacSha256` and `ddcore.crypto.timingSafeEqual` verify a signature over that body,
  so an inbound webhook can be authenticated inside the app. A body that is not JSON no longer fails
  the call unless it is sent as `application/json`: `args` is empty and `rawBody` has the payload.
  See *Inbound webhooks* in `controller-api`.
- **`uniqueKey` for `ddcore.enqueue`.** While a job with the same key is still `queued`, the call
  queues nothing and returns that job's id, so "process session X" can be enqueued per event
  without stacking jobs. A claimed job frees the key, so work arriving mid-run queues a new one.
  Enforced by a partial unique index, safe under concurrent requests.
- **PocketID provisioning.** Set `DDCORE_OIDC_<ID>_API_KEY` on a provider of kind `pocketid`
  (`DDCORE_OIDC_<ID>_KIND`, the default for the id `pocketid`) and an invitation of a System User
  creates the account in PocketID — or reuses the one with that address — and mails PocketID's
  one-time link to register a passkey (`core.invite_sso`) instead of a link to choose a
  password. A mapped group PocketID does not have yet is created there on first use, by the
  invitation or by the role push-back, so a fresh PocketID needs no groups made by hand; a group
  that cannot be created fails with a message naming it. A refusal from PocketID fails the
  invitation with its reason. A Website User is still invited with a password.
- **Group → role mapping for single sign-on**: `auth.sso.<provider>.groupRoles` in
  `ddcore.json` (and `groupsClaim`, default `groups`). Each sign-in sets the mapped roles from the
  provider's groups and leaves every other role alone; a missing claim changes nothing, and
  Admin is never touched. With provisioning on, saving a User pushes its mapped roles back to
  PocketID as groups, through a retried job (`core.services.idp.sync`, queue `idp`). The
  `groups` scope is requested automatically. Refused at load: a policy for an unknown provider,
  a group with no role, and `Admin`, `Guest` or `All` as a mapped role.
- **The User form asks before following a disable, enable or delete to PocketID**, and calls
  `core.services.users.setProviderDisabled` on a yes; nothing is disabled there on its own.
  `accountStatus` now also returns the provisioning `provider`.
- **`afterDelete(frm)`** in `defineForm`, run after the form's Delete succeeds while `frm.doc`
  still holds what was deleted.
- `ddcore doctor` probes the PocketID admin API, warns about mapped roles missing on the site,
  and lists the mapped groups PocketID does not have yet as created on first use
  (`groupsToCreate` in `--json`).

### Changed

- **`ddcore user invite` takes the same path as the desk's *Save and invite***: it now records
  `account.invite`, refuses an address that already has a User with the usual message, and
  provisions PocketID when that is on.
- **`whitelisted(fn, { methods })` is now enforced.** It was declared but ignored; a verb outside the
  list answers 405 with an `Allow` header. An app that listed `methods` and relied on the other verb
  still working must add it to the list.
- **`ddcore mcp` starts without the database.** An unreachable Postgres no longer kills the
  process before the handshake, which an MCP client only reported as a closed connection. The
  meta, scaffold, i18n and docs tools keep working; the tools that need the database answer
  with an error that says so, and connect on their own once Postgres is up — no reconnecting
  the client. `engine.Config.DeferDB` and `Engine.Connect` are what an embedder uses for the
  same.

## 0.21.12 — 2026-09-29

### Added

- **`defineListView({ filtersCollapsed: false })`** (#32) opens a DocType's filter card for a user
  who has not chosen yet.
- **`linkOrderBy`** (#33) on a DocType sets the order of its Link dropdowns and Table
  MultiSelect pickers — `"field [asc|desc], ..."` — without touching the list's `sortField`.
  `extendDoctype` can set it on another app's DocType, `User` included. See `fieldtypes`.
- **`calendar.newOptions`** (#34) sets what the **+** in a calendar day's corner creates: records of
  other DocTypes, each with its date `field` (and optionally an `endField`) set to the day. One
  option links straight to its form, several open a menu, and each shows only when the user may
  create its DocType — so a read-only or virtual DocType that gathers others' records gets a
  **+** too. See "Views" in `docs/agent/form-api.md`.
- **An `any` filter item ORs groups of filters** (#35) inside `filters`:
  `[["status", "=", "Open"], { any: [[["priority", "=", "High"]], [["due_date", "<", today]]] }]`.
  A row matches when every filter of one group does. It works in the list API, `ddcore.db.getList`
  and `count`, and exports. `or_filters` still holds one OR group, and a list's search already
  takes it. A filter inside a group is checked against field permissions like any other. See
  "Filters" in `docs/agent/controller-api.md`.

### Changed

- **Clicking a calendar day lists what the grid draws on it** (#35). With `endField`, the list
  shows every record whose bar covers the day. That includes a record that started earlier and
  ends on or after it, where the click used to list only the records starting that day. The day
  is its own filter: `?calendar_day=2026-09-29` in the URL, a removable **Day** in the filter
  card, counted by the **Filters** button. It replaces `?start_date=2026-09-29`, which was an
  equality on the calendar's `field`. Without `endField` it still lists the records whose
  `field` falls on the day.

- **A list's filter card starts hidden behind a Filters button** (#32) in the list header. The
  button's badge counts the filters in force, so a hidden filter never goes unseen, and each
  user's show/hide choice is kept per DocType in the browser. See `defineListView` in
  `docs/agent/form-api.md`.
- **A Link dropdown lists its options by title, A to Z** (#33), and the same holds for a
  Table MultiSelect's picker. They used to follow the target's `sortField`, or `modified desc`.
  Typed text puts the options whose id or title equals it, then starts with it, first. Text
  compares ignoring case and accents. A DocType without a `titleField` keeps its `sortField`
  order; set `linkOrderBy` to choose another.

## 0.21.11 — 2026-09-29

### Added

- **A Table's `onChange` knows which child field changed** (#30): it receives a fifth argument,
  `changed`, the child fieldnames an edit in the grid, the row dialog or `frm.setRowValue`
  changed. `grids.<table>.onChange.<child field>(frm, row)` runs for a change of that one field,
  before the table's `onChange` — next to `grids.<table>.onCellClick`. See
  "Grids: row changes and cell clicks" in `docs/agent/form-api.md`.
- **`hideLabel: true` keeps a field's label off the form** (#31). Screen readers still read it,
  and it still names the field in a grid's CSV/XLSX export, its row dialog and error messages.
  It can be set from `extendDoctype` and `frm.setDfProperty`. On a Section or Tab Break it fails
  validation.

### Changed

- **A section with no heading whose only visible field is a Table or a Report has no card**
  (#31). The grid's own card frames it, so a `Tab Break` followed by one Table no longer nests
  one frame in another. Add `hideLabel: true` to the grid and the tab shows just the grid. See
  "Form grids" in `docs/agent/fieldtypes.md`.

- The `ddcore running` startup log line now carries the core `version`, so a deploy's logs say
  which release is serving.

## 0.21.10 — 2026-09-28

### Added

- **`Autocomplete` fieldtype**: free text with suggestions, stored in a text column. `options`
  lists the suggestions (a list, or one per line) and never restricts the value; the server
  trims outer spaces. The desk's combobox filters ignoring case and accents, and a form script
  replaces the suggestions with `frm.setDfProperty(field, "options", list)`. Changing a `Data`
  field to `Autocomplete` needs no migration, and the first save's trim is not recorded as a
  Version.
- **`Barcode` fieldtype**: text drawn as a barcode, stored in a text column. `options` is the
  symbology — `Code128` (the default), `EAN-13` or `QR` — and the server validates the value for
  it: printable ASCII up to 80 characters, 12 or 13 digits (a 12-digit EAN-13 gets its check
  digit), or up to 1000 bytes. The desk previews the code under the text box and, where the
  browser has `BarcodeDetector` on a secure origin, scans it with the camera. The standard print
  layout draws it as SVG, and print templates get `b.barcode(value, symbology, title)`.
  `GET /api/barcode?symbology=&value=` returns the SVG to any signed-in user, Website Users
  included.
- **`Signature` fieldtype**: a signature drawn on a pad with a finger, a pen or the mouse, stored
  as a PNG data URL in a text column — the value Frappe stores. The server accepts only a PNG
  data URL of at most 64 KiB and 2000×1000 pixels. The form commits each stroke cropped to the
  strokes, shows a stored signature as an image with "Sign again", and a grid opens the row
  dialog for it instead of editing the cell. Versions record a `sha256:` marker instead of the
  image; lists, grid exports and child-table print cells say "Signed"; Data Import does not take
  it. The standard print layout draws it, and templates get `b.signature(dataUrl, title)`. Being
  large, it is refused as `unique`, `searchIndex`, `inStandardFilter`, `inListView` (outside a
  child DocType), `titleField`, `sortField`, `searchFields`, `linkSubtitle`, `uniqueKeys` or
  `gridSort`.
- **`Geolocation` fieldtype**: points, lines and polygons drawn on a map, stored as a GeoJSON
  FeatureCollection in a `jsonb` column. The server accepts a FeatureCollection, a Feature or a
  bare Point, MultiPoint, LineString or Polygon, and stores one canonical shape: positions
  `[lon, lat]` rounded to 7 decimals, no altitude, no properties, rings closed, at most 500 shapes
  and 64 KiB — so a document saved untouched records no Version. Hooks read it as an object
  (`doc.area.features`), typed `GeoFeatureCollection` (new `Geo*` types in `@ddcore/sdk`). The form
  loads Leaflet on demand, with Point, Line, Polygon and Delete tools and "Use my location"; lists,
  exports, history and print show a summary (`lat, lon`, or "2 points, 1 polygon") — print draws no
  map. Data Import takes GeoJSON or `lat; lon`. Tiles come from the new `DDCORE_MAP_TILE_URL` and
  `DDCORE_MAP_ATTRIBUTION` env vars, served to the desk in `/api/boot` as `site.map`; the default,
  OpenStreetMap's own server, is not meant for production traffic. Refused in the same places as a
  Signature, except `inListView`.

### Changed

- Translated metadata carries `optionLabels` only for a `Select`. A `Duration`'s display flags
  no longer get labels, and an `Autocomplete`'s suggestions are never translated.

### Fixed

- **API key requests no longer run Argon2 on every call** (#29). A key's secret is hashed once
  while its key row is cached (60 s); after that a matching secret is recognised by a
  per-process HMAC digest and skips the 64 MiB hash, and concurrent checks of the same secret
  share one hash. Revoking or disabling the key or its user
  drops the verification with the row, so revocation latency is unchanged, and a wrong secret
  is still hashed and counted by the failure brake. `last_used` is stamped when the hash runs,
  so at most once a minute per key.
- **Concurrent Argon2 computations are bounded** to `max(2, GOMAXPROCS)` per process. A burst
  of sign-ins or API-key checks queues for a slot instead of allocating 64 MiB each, which
  could OOM-kill the process under load.

## 0.21.9 — 2026-09-28

### Added

- MCP tool `extend_doctype` writes `extensions/<snake>.extend.ts` — fields, per-field and
  DocType property overrides, extra roles, and optionally the `.form.ts` beside it — the way
  `scaffold_doctype` writes a DocType. It checks the host is in `requires` before writing, loads
  the file before returning, and removes it again when the meta refuses it, so a clash never
  leaves the site unable to load. It only creates: an existing extend file is edited by hand.

## 0.21.8 — 2026-09-28

### Added

- A delete leaves a trace (#28). Every deleted document gets a final Version whose new
  `deleted` field is checked and whose `data` is `{"deleted": {…}}`: the document as it was,
  child rows included, without its Password, Vault and secret fields. A `doc.delete` Audit Event
  records who deleted it and points at that Version. This applies to every DocType except
  Version, Error Log, Email Delivery and Webhook Delivery, which are only audited. List Version
  with **Deleted** checked to see what was deleted. There is no undelete. See "What a delete
  leaves behind" in `controller-api`.

### Changed

- Deleting a document no longer deletes its Versions: a `trackChanges` history now ends with the
  deletion instead of disappearing. Only a System Manager reads the Versions of a deleted
  document. A document later created under the same id shows other readers its own Versions
  only. `migrate` adds the `deleted` column to `tab_version`.
- Versions are stamped with `clock_timestamp()`, so two written in one transaction sort in the
  order they happened.

## 0.21.7 — 2026-09-28

### Fixed

- Deleting a User ends their sessions and clears their cached roles immediately, as disabling
  one already did; the delete is recorded as an `account.delete` audit event.
- Deleting or renaming a document no longer fails with `column "…" does not exist` when a
  `computed: true` Link of another DocType points at its DocType: computed fields have no column,
  so the link check and the rename skip them (#26).
- A filter on a Check field accepts `1`, `0`, `"1"`, `"0"` and `"true"` as well as a boolean —
  in a workspace number card's `filters`, `getList`, `count` and the REST API — instead of
  failing to encode an integer into a `boolean` column (#27).

## 0.21.6 — 2026-09-28

### Added

- **The User form sends invitations.** Saving a User only writes the row, so nobody was told an
  account existed and the desk had no way to reach `users.invite`. A new User form now offers
  **Save and invite**; a saved, enabled User offers **Resend invitation** while it has no
  password and **Send password reset** once it does. On the `log` transport the link is shown to
  be passed on by hand. `core.services.users.accountStatus({ user })` answers
  `{ user, hasPassword }` for it.

### Changed

- **`frm.setDfProperty` types its property.** `prop` is now `DfProperty` — a key of `FieldDef`
  or `cannotAddRows`/`cannotDeleteRows` — so a misspelt property is a type error instead of a
  silent no-op. `description` is documented among them: a form script may rewrite a field's help
  text at runtime (#24).

### Fixed

- **A form script named after its `.doctype.ts` loads again.** The engine and the asset route
  looked only for `<Snake(name)>.form.ts`, so "TagOne Settings" wanted `tag_one_settings.form.ts`
  and a `tagone_settings.form.ts` beside `tagone_settings.doctype.ts` was never loaded —
  `formApps` came back empty and no `defineForm` ran. A script is now also found by the stem of
  its DocType's own file (#25).

## 0.21.5 — 2026-09-28

### Changed

- **`ddcore init` and `ddcore deploy` write the Dockerfile on the minor series.** A new site
  starts `FROM ghcr.io/jrvidotti/ddcore:0.21` rather than `:0.21.0`: the release workflow moves
  that tag to every patch, so each deploy takes the series' fixes without a commit, and it never
  leaves the `ddcore` range `ddcore init` writes. An existing Dockerfile is left alone — change
  `:0.21.0` to `:0.21` by hand to follow the series, or keep the exact tag to pin one release.

## 0.21.4 — 2026-09-28

### Fixed

- **A reload reinstalls the scheduler's entries.** A `scheduler` block added or changed while
  `ddcore dev` ran was loaded — `ddcore jobs scheduled` listed it — but the running scheduler
  kept the entries it built at boot, so the new entry never fired until the server restarted
  ([#23](https://github.com/jrvidotti/ddcore/issues/23)). Every successful reload now rebuilds a
  running scheduler: the file watcher, and the MCP tools that reload (`reload`, scaffolding,
  translations).

## 0.21.3 — 2026-09-28

### Fixed

- **`ddcore.db.getSingleValue(doctype, field)` returns the Single's value again.** Since 0.17
  renamed a Single's id to `"singleton"` it looked up a row named after the DocType and answered
  `null` for every field ([#22](https://github.com/jrvidotti/ddcore/issues/22)). It now reads the
  `singleton` row, answers the field's default before the first save (as `getDoc` does), and
  refuses a DocType that is not a Single. It is now described in the controller API.

## 0.21.2 — 2026-09-28

### Changed

- **The changelog is split by minor series.** `CHANGELOG.md` and the MCP resource
  `ddcore://changelog` now carry `Unreleased` and the current series only; each older series is
  `docs/changelog/<minor>.md`, served as `ddcore://changelog/<minor>` and published on the docs
  site under Changelog. `whats_new` still reads across every series, so an app several minors
  behind hears about all of them.

## 0.21.1 — 2026-09-28

### Changed

- **`ddcore deploy railway` writes the Railway project as Infrastructure as Code**,
  `.railway/railway.ts` (with the `package.json` of its SDK), instead of `railway.json`, which
  Railway reads only until 2026-12-01. The file declares the site's service — its GitHub source
  taken from the checkout's `origin`, the `/api/ready` health check — and a PostgreSQL whose
  `DATABASE_URL` it wires in, with every secret as `preserve()`; it is applied with
  `railway config plan` / `apply`. **Upgrade:** a site with `railway.json` runs
  `railway config migrate --apply --delete-files`, then declares the service's `source` (a
  plan without it disconnects the repository) and its variables as `preserve()`; the
  deployment guide's Railway section walks through it. The generated `.dockerignore` now leaves
  `.railway` out of the build context.

### Fixed

- A `make build` stamped itself after the rolling `edge` tag (`edge-1-g…`), which is not a
  release, so it stopped enforcing `ddcore` ranges; it now describes itself from `v*` tags only.

## 0.21.0 — 2026-09-28

### Breaking

- **`"dev"` in `ddcore.json` is retired.** Committed, it put every environment — production
  included — in development mode, and nothing could turn it off: app assets rebuilt on every
  request, webhooks allowed over plain `http://`, mail redirected to `DDCORE_MAIL_DEBUG`.
  Development mode is now `ddcore dev`, or `DDCORE_DEV=1` for another command. A file that
  still has the key loads, logs `ddcore.json "dev" is ignored since 0.21.0 …`, and loses it on
  the next save; `ddcore init` no longer writes it. **Upgrade:** delete `"dev"` from
  `ddcore.json`, and set `DDCORE_DEV=1` in `.env` where a command other than `dev` should run
  in development mode.

### Changed

- **`ddcore start` migrates before it serves**, as `dev` always did, so a deploy never runs
  against a schema its apps do not declare. **Upgrade:** a deployment that migrates in a step
  of its own sets `DDCORE_AUTO_MIGRATE=0` (or passes `--auto-migrate=false`).
- `main`'s rolling build is now the prerelease **`edge`**, not a release named `latest`, so
  GitHub's latest release — what `install.sh` installs by default and `ddcore doctor`'s update
  check reads — is always a tagged version. `VERSION=edge` installs the rolling build.

### Added

- **The official image `ghcr.io/jrvidotti/ddcore:<version>`** (also `:<X.Y>` and `:latest`),
  linux amd64/arm64, published by every tagged release: the binary on Alpine with
  `pg_dump`/`pg_restore` 17 and the timezone data, `WORKDIR /app`, uploads in `/data`, `$PORT`
  honoured, `CMD ["ddcore", "start"]`. A site's Dockerfile is `FROM` it plus `ddcore.json` and
  `apps/`, and its tag is the site's pin.
- **`ddcore init` writes a `Dockerfile` and `.dockerignore`**, on the image of the release
  that created the site, and **`ddcore deploy docker|railway`** writes them into an existing
  site; `railway` adds `railway.json` (Dockerfile build, `/api/ready` health check, restart on
  failure) and lists the variables to set. Existing files are left alone.
- **`DDCORE_ADMIN_PASSWORD`**: the first migration gives Admin this password instead of
  generating one and printing it to the log, which a container has no console to read. It never
  replaces a password Admin already has, and a password the site's policy refuses fails the
  migration by name.
- **Replicas are safe against one database.** A migration takes an advisory lock, so replicas
  that each migrate on boot take turns instead of racing; every scheduler still fires, but a
  cron entry is enqueued once per minute however many processes run it (the ledger is the new
  internal table `ddcore_scheduler_tick`).
- `install.sh` verifies the archive against the release's `SHA256SUMS`, and takes `VERSION`
  with or without the leading `v` (`0.21.0` or `v0.21.0`).
- `docs/guide/deployment.md` covers the image, Docker Compose and Railway, and no longer
  mentions a `ddcore run` command, `DDCORE_ENV`, `/api/v1/health` or port 8090, none of which
  exist.

### Fixed

- `ddcore jobs work` stops gracefully on SIGTERM — how a container is stopped — putting the
  jobs it was running back, instead of being killed mid-job.
- `ddcore start -h` and `ddcore dev -h` name their own command, not a `serve` that does not
  exist; the boot log reports the development mode in effect.

<!-- #endregion releases -->
