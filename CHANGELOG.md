# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Fixed

- The site is named by its own app, not by a library that app `requires`: a required app loads
  first, so it used to take the Desk's title, `desk.home` and `desk.logo`. The apps nobody
  requires now come first for all three, and `/api/boot` carries the result as `site.home` and
  `site.logo` (#54).
- A `beforeEach`, `afterEach` or `beforeAll` written outside any `describe` applies to the tests of
  its own file. It used to run around every test of every file of the site, so a fake installed by
  one app's test file was in place during another app's tests. Tests run file by file as a result:
  a file's top-level tests, then its `describe` blocks (#48).

## 0.22.1 — 2026-09-30

### Added

- `defineListView({ actions })`: app buttons over the rows selected in the List or Cards view,
  shown next to "Delete (n)" as `label (n)`. An optional per-row `condition` picks the rows an
  action applies to; `onClick(ids, list)` receives their ids and loaded values, and the list clears
  the selection and reloads once it settles. See `form-api` (#53).

## 0.22.0 — 2026-09-30

### Added

- Desk scripts receive `ddcore.publish` events: `frm.onRealtime(event, handler)` for as long as
  the form is open, and `ddcore.realtime.on(event, handler)` / `off` anywhere else. `publish`
  honours `{ doctype, id }`: the event reaches only sessions that may read that document (#49).

- `ddcore.files.save` stores bytes server code holds or downloads (`content`,
  `contentBase64` or `fromUrl` with headers, a size limit and a timeout) as a `File`, with
  the rules of an upload. The row is on the current transaction, and a rollback deletes
  the bytes too. `ddcore.files.presign(fileUrl, { ttl })` returns a presigned URL for a
  third party with no session; it needs the s3 storage backend. See `storage` (#50).
- An upload whose transaction rolls back after the `File` row was inserted no longer
  leaves its bytes behind in the store (#50).

- `ddcore.http` takes `responseType: "base64"`, which returns a binary body (an image, an audio
  file) base64-encoded instead of corrupted as text, and `maxBytes`, the largest body accepted
  (default 10 MiB) (#51).

### Changed

- `ddcore.publish` throws on an event name outside letters, digits and `_ . : -`: the name is
  written into the SSE stream as is, and a line break in it could forge events (#49).
- A `ddcore.http` response larger than its limit now throws instead of being silently cut at
  10 MiB. Pass a larger `maxBytes` where a bigger body is expected (#51).

### Fixed

- `ddcore i18n extract` collects `_()` / `__()` inside a template literal's `${…}`, and a template
  nested in an interpolation no longer ends the outer one and hides every key after it in the file.
  Both used to pass `--check` silently. Run `ddcore i18n extract` again: keys that were missed now
  show up as missing and need a translation (#52).

<!-- #endregion releases -->
