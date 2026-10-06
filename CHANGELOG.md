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
