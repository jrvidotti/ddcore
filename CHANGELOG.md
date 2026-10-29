# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.30](docs/changelog/0.30.md), [0.29](docs/changelog/0.29.md), [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

## 0.31.9 — 2026-10-29

### Added

- **A child table can be a list's standard filter.** `inStandardFilter: true` on a `Table` or
  `Table MultiSelect` whose child has exactly one Link shows a Link filter in the list's filter
  card, and the list keeps the parents with a row holding the value chosen
  (`["<Child>.<link>", "=", value]`). `migrate` refuses it on a table whose child has no Link or
  more than one. The User list uses it: it is filtered by role.

## 0.31.8 — 2026-10-29

### Changed

- **Desk: the notification center opens on unread notifications.** The **Show** selector starts
  at **Unread** instead of **All**, and its options are translated as a filter (pt-BR: "Não
  lidas", "Lidas", "Todas") rather than with the per-row status words. With nothing unread, the
  page says "No unread notifications".

## 0.31.7 — 2026-10-29

### Added

- **An administrator can impersonate a user**: the User form's **Impersonate** button (`ddcore.impersonate(user)` in a desk script, `POST /api/impersonate`) makes the session work as that user — their desk, or the portal for a Website User — with a banner to stop (`POST /api/impersonate/stop`). Admin and System Manager only; never Admin, a disabled user, or inside another impersonation; under tenancy, the spaces `ddcore.runAs` keeps. Reads and writes are the user's, and every audit event and Version written meanwhile carries `impersonated_by`; `account.impersonate` and `account.impersonate_stop` record the start and the end. Minting or revoking the user's API keys, changing their password and revoking their sessions are refused meanwhile. The desk's `ddcore.session` gains `impersonatedBy`. See "Impersonation" in `auth`.
- **A notification rule can say when its occurrences stop being relevant** (#140). A
  `resolveWhen: (doc, before) => boolean` next to `condition` is evaluated on every update, submit
  and cancel of the document, whatever the rule's trigger (date rules included). When it holds,
  every recipient's open occurrences of that rule on the document are marked read and resolved in
  the document's transaction, each inbox refreshes after the commit, and an email of theirs still
  queued is not sent (its delivery ends `Failed`, `Notification resolved`). A later match writes a
  fresh occurrence. Server code does the same with
  `ddcore.notifications.resolve({ rule, doctype, id }, { ignorePermissions? })`, for an app's rule
  or the core's `assignment` and `share`. `ddcore migrate` adds the `resolved` column to the
  internal notification table. See `notifications`.

### Fixed

- **Notifications show when they were created** (#139). `creation` in `GET /api/notifications`
  and `PATCH /api/notifications/{id}` was always Go's zero time (`0001-01-01T00:00:00Z`), so the
  desk's notification center showed `31/12/1` on every row, for every rule. The date-rule sweep
  also stopped recognising the dates it had already notified and re-claimed them on every pass;
  the claim kept it from sending twice, but each pass locked every past-due document again.

## 0.31.6 — 2026-10-29

### Fixed

- **Desk: a `Time` field can be typed into** (#138). It was a native time input that opened the
  browser's picker on every click, and in Chrome the picker took the keyboard, so `08:00` had to
  be picked hour by hour. It is now a masked text input like `Date` and `Month`: digits only on a
  24-hour clock (`0800` → `08:00`, seconds shown only when the value has them), ↑/↓ step the part
  under the caret, and a list of half-hour slots, **Now** and **Clear** sit behind the clock
  icon (or Alt+↓) instead of opening on the click. The same in a child table's row. `Datetime`
  no longer forces its native picker open on a click either, so its segments can be typed.

## 0.31.5 — 2026-10-27

### Added

- **Server code can replay a webhook delivery**: `ddcore.webhooks.replay(delivery, { ignorePermissions? })` sends a `Sent` or `Failed` delivery again, with the same `webhook-id` and body, queued on the caller's transaction like `emit`, and returns `{ delivery, status: "Queued" }`. Without `ignorePermissions` it is the Replay button's check (a System Manager without access scopes); with it, the app's rule decides. Either way it writes the `webhook.replay` audit event. See `webhooks`.

## 0.31.4 — 2026-10-24

### Added

- **A list can filter a date by period**: `defineListView({ rangeFilters: ["due_date"] })` shows each Date or Datetime field named as two day pickers in the filter card, `<label> from` and `<label> to`, either of which may stay empty. The URL carries `field=from..to` (one side may be empty), and a bare day still means that day alone, so existing links keep working. A Datetime's days run from the first instant to the last in the site's timezone. See `form-api`.

## 0.31.3 — 2026-10-22

### Added

- **Server code can read a stored File back, and a print can show a private image** (#137). `ddcore.files.read(file, { maxBytes?, ignorePermissions? })` returns `{ base64, contentType, size, fileName }` for a File named by its id or its `file_url`, on the local and the s3 backend alike; `maxBytes` defaults to 50 MiB. `ddcore.files.sha256(file, { output?, ignorePermissions? })` streams the stored bytes through SHA-256, giving what `sha256sum` prints for the file. Both need read on the File as a download does (a private file follows `/private/files`, a public one is anyone's). `ddcore.crypto.sha256(data, { inputEncoding })` hashes `"base64"` or `"hex"` input as the bytes it encodes (`"utf8"` stays the default). The print block `b.image(file, { title?, maxHeight?, maxWidth? })` prints the image a File holds, read by the server as the user printing and inlined as a data URL, so a private photo reaches a PDF Gotenberg renders without a session; a file the user may not read, over 10 MiB, or that is not a PNG, JPEG, GIF or WebP image prints as nothing. See `storage` and `print`.

### Changed

- **The standard print layout inlines `Attach Image` fields** (#137) through `b.image`, instead of an `<img>` pointing at `/private/files/…`, which a PDF renderer could not fetch. An image the printing user may not read, or whose bytes are not an image, is now left out rather than printed as a broken picture.

## 0.31.2 — 2026-10-22

### Added

- **Server code can render a print template to a PDF `File`** (#136). `ddcore.print.pdf({ doctype, id, format?, letterHead?, language?, pageFormat?, landscape?, filename?, ignorePermissions? })` prints the document as `GET /api/print/{doctype}/{id}/pdf` does, and stores the PDF as a private `File` attached to the document, on the current transaction, so `ddcore.sendMail({ attach: [pdf.id] })` can mail it from a job. Printing needs `read` and attaching needs `write` on the document, unless `ignorePermissions`; the File is named `<format>-<id>.pdf` by default. Without a PDF renderer it throws `UnavailableError`. See `print`.

### Fixed

- **A binary newer than the database's last migration says so** (#135). `ddcore migrate --dry-run` lists the framework's own schema changes — such as the `ddcore_vault.key_id` column 0.31.1 adds — under `framework (before the patches)` instead of answering `nothing to do`; `migrate` counts them in its `ok:` line and records them in `ddcore_migration`, and `ddcore doctor` and `ddcore dev` without `--auto-migrate` report them as pending DDL. `ddcore vault rekey` (MCP: `vault_rekey`), the doctor's vault census and the vault reads and writes stop with `The vault has not been migrated to this version: run ddcore migrate` instead of Postgres's `column "key_id" does not exist`, and a re-key that cannot start records no Audit Event.

## 0.31.1 — 2026-10-22

### Added

- **The vault's master key can be rotated** (#134). `DDCORE_SECRET_KEY_PREVIOUS` holds the old key while a new one is deployed in `DDCORE_SECRET_KEY`: a secret that does not open under the current key is read with the previous one, and every write uses the current one. `ddcore vault rekey` (MCP: `vault_rekey`) then re-encrypts every secret still under the previous key, in every tenant, in batches of one transaction each; `--dry-run` counts the secrets under each key, a secret that opens under neither stops it by name (never by value), and a run is one `vault.rekey` Audit Event with counts only. `ddcore doctor` warns while secrets remain under the previous key, says when it can be removed, and names secrets no key opens. Each `ddcore_vault` row now records a fingerprint of the key that encrypted it (`key_id`, added by `migrate`); rows written before are stamped by the first `rekey`. `ddcore.secret("key_previous")` is refused, as `ddcore.secret("key")` is. See `vault` for the procedure and `backup` for archives taken before a rotation.

## 0.31.0 — 2026-10-19

### Fixed

- **The history filter also searches removed rows.** Searching a document's full history for a value that only a removed child-table row held found nothing; added, modified and removed rows are all matched now.
- **Security: a `report` right no longer widens a list beyond what `read` allows.** Listings counted a permission row with `report` (and no `read`) as a read grant, while opening a document did not. A role with `report` but not `read`, once one document of the DocType was shared with the user, listed every document of it, and every child row of its tables; a `report` row beside an `ifOwner` read listed other users' documents too. Lists (including the tree view and virtual DocTypes) now follow the same rule as reading one document: only `read` rows grant it, `report` alone grants nothing to list. If a role was meant to list a DocType, give it `read`.
- **Security: a child DocType lists only the rows of parent documents the user can list.** A listing of a child DocType (`getList`, `count`, `GET /api/resource/<child>`) checked only that the user could read *some* parent DocType, so a user limited by User Permission scopes listed the rows of every parent document out of scope when the rule was `applicable_for` the parent or the child table had no `Link` to the scoped DocType (line items without a `company`, say); the parent controller's `permissionQuery` was ignored, and so was the permission level of the `Table` field holding the rows. A row is now listed only when its parent document passes everything a list of the parent applies for the user (roles, shares, scopes, `permissionQuery`, portal), its `Table` field is readable at its level, and its own `Link` fields pass the rules for the parent's `applicable_for`; a parent DocType the user reads only as owner (`ifOwner`) still lists no child rows at all. Reading one row directly checks the field level and those row rules as well, and also the rules without `applicable_for` against the row's own `Link` fields. Upgrade note: users limited by scopes, or listing the children of a parent whose controller has a `permissionQuery`, now see only the rows of parents they can list; rows of a `Table` field the user cannot read at its permission level are no longer listed or readable directly; Admin and `ignorePermissions` reads are unchanged.
- **An error raised while checking a permission is reported as that error.** A controller `hasPermission` that throws (or a failed lookup behind the check) made submitting on insert, deleting, renaming, amending and merging answer "No permission"; the error now surfaces as itself, as it already did for reading, saving and creating.

<!-- #endregion releases -->
