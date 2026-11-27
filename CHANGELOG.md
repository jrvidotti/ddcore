# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.32](docs/changelog/0.32.md), [0.31](docs/changelog/0.31.md), [0.30](docs/changelog/0.30.md), [0.29](docs/changelog/0.29.md), [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

## 0.33.4 — 2026-11-27

### Added

- **`ddcore.users.passwordStatus(user)` for app server code (#175).** It answers
  `{ hasPassword, elsewhere }`: whether the person has a password, and whether they also have a
  User row in another space of the site, never which. Since 0.32 neither fact was readable by
  app code.
  - `user` is an id or an e-mail, matched as inserting a User matches it, so an app can ask
    before it inserts. One that creates Users from an external directory checks `elsewhere`
    so it does not join two different people who share an address.
  - Anyone may ask about themself. About another User here, the caller needs read on User.
    About an address with no User here, the caller needs create on User, and `hasPassword` is
    then always `false`.
  - Code that read `password_hash` with `ddcore.db.getValue("User", …)`, which fails since
    0.32 with `Unknown field: password_hash`, reads `hasPassword` instead.
  - The SDK exports `PasswordStatus`. See `controller-api` and `auth`.

## 0.33.3 — 2026-11-27

### Fixed

- **`ddcore test` no longer writes to the site's bucket (#174).** On a site with
  `DDCORE_STORAGE=s3`, every test that saved or deleted a file went to the real bucket. That made
  a large suite about three times slower, and left objects there that the rolled-back
  transaction could not take back out.
  - `ddcore test` and the MCP `run_tests` now keep the tests' files in a temporary local
    directory, removed at the end, whatever `DDCORE_STORAGE` says.
  - `ddcore test` prints the backend it uses on its first line (`storage: …`).
  - A `.env` with `DDCORE_S3_FALLBACK_PREFIX` no longer has to be edited to run the suite locally.
  - `ddcore test --site-storage` uses the site's storage, for a test that needs the bucket.
  - See `storage` and `cli`.

## 0.33.2 — 2026-11-27

### Fixed

- **The report page no longer runs `execute` without the filters' defaults (#173).** It opened by
  calling `execute({})` only to read the report's meta. A report that refused the empty filters
  (`ddcore.throw("Set the period")`) left the page with an error and no filter card to set them.
  - The page now reads the meta from `GET /api/report/<name>?meta=1`, which does not run
    `execute` and checks the same permissions.
  - The first run gets the defaults and the URL's filters. A run that fails keeps the filter card.
  - A report that copied its filters' defaults into `execute` as a workaround can drop the copy.
  - See `report-api`.

## 0.33.1 — 2026-11-27

### Added

- **A report chart can be a heatmap (#172).** `type: "heatmap"` draws a matrix: a column per
  label and a row per dataset, each cell shaded by its value. A weekday × hour traffic report
  reads its peaks at a glance.
  - The scale runs from the lowest to the highest value, or from `min` to `max`. Its hue is
    `color` (`blue` by default), mixed into the theme's surface.
  - `null` is an empty cell. A cell prints its value when it fits, the tooltip names the row
    and the column, and a legend shows the scale.
  - `drill` is one array per row, `drill[row][col]`, so a click on a cell opens that slot.
  - The SDK's `ChartData` and `ChartDef` take the new `type`, plus `color`, `min` and `max`.
  - See `report-api`.

## 0.33.0 — 2026-11-25

### Breaking

- **Messages shown by the desk are text, not markup (#170).** This is a security fix: a message
  built from a document's data (`__("Did the lookup show {0}?", [frm.doc.full_name])`) ran any
  `<img onerror=…>` someone had typed into that field, for whoever clicked the button. The change
  covers `ddcore.ui.toast`, `confirm`, `msgprint`, `alert`, a dialog's `message`, and the server's
  `ddcore.throw` and `ddcore.msgprint`.
  - Line breaks in a message still show.
  - Upgrade path: a message that really carries markup passes `{ html: true }` (on a dialog spec,
    next to `message`) and escapes every value in it with `escapeHtml`, now exported from
    `@ddcore/desk-sdk`. A message that only escaped values to be safe can drop the escaping:
    `&amp;` would now show literally.
  - The server's `ddcore.bold(v)` is deprecated and returns `String(v)`.
  - See `form-api`.

### Changed

- **A failed single sign-on goes back to the page that started it (#171).**
  - A sign-in started under an app's static site (`start?redirect=/atleta/entrar`) fails back to
    that page with `sso_error=<code>` added to its own query, so the site shows the failure
    instead of the desk's `/login`.
  - This covers a missing state cookie too, as from an installed PWA's own cookie jar. The state
    in the query still names its sign-in, and is read without being spent.
  - A desk or portal sign-in still fails to `/login?sso_error=<code>`, which now keeps
    `&redirect=…`, so a retry goes where the first attempt was going.
  - When the start cannot be traced (an expired state), the new `auth.sso.<id>.errorRedirect`
    names the page, default `/login`.
  - The codes are unchanged. See `auth`, "Single sign-on".

<!-- #endregion releases -->
