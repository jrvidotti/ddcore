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

### Fixed

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
