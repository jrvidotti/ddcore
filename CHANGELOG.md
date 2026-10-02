# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Added

- `ddcore.datetime.dateDiff(a, b)` and `ddcore.datetime.monthDiff(a, b)` on the Desk, with the
  semantics of `ddcore.utils.dateDiff` and `monthDiff` on the server, so a form script can show a
  day count the controller computes: `dateDiff("2026-05-10", "2026-05-01") === 9`. See "Dates and
  times" in `form-api` (#58).
- The Tree view has a search box. While there is text, the tree shows only the nodes that match
  — by `id`, `titleField` and `searchFields`, as a Link search does — and the ancestors that lead
  to them, every branch open and the matches highlighted; the text is the list's `?q=`. Nothing
  to declare in an app. On the API, `GET /api/tree/{doctype}?search=` answers those nodes, a
  match flagged `"match": true`. See "The tree view and `/api/tree`" in `trees` (#59).

## 0.23.5 — 2026-10-01

### Changed

- Desk addresses carry a DocType, a workspace or a report without the spaces of its name:
  `/app/Training%20Class` is now `/app/TrainingClass`, and `/app/<Workspace>/report/Open%20Tasks`
  is `/app/<Workspace>/report/OpenTasks`. Nothing to change in an app: the name as it is still
  opens and is redirected, so a `route:`, a `ddcore.route(...)` call and a link in a mail already
  sent keep working. Record ids and `/api` paths are untouched. See "Navigation" in `form-api`.
- `ddcore.docUrl(doctype, id)` returns the address in that form (`…/app/SalesOrder/SO-1`).
- Two DocTypes, two workspaces or two reports whose names differ only by spaces are refused when
  the site loads, naming both: they would share a Desk address. (Two such DocTypes already shared
  a table.)

## 0.23.4 — 2026-10-01

### Fixed

- The short route keeps its query string and its hash when the Desk redirects it to the
  workspace route. `/app/<DocType>/new?field=value` opened the new form with the field empty and
  `/app/<DocType>?field=value` opened the list unfiltered, because the redirect was built from
  the path alone. `form-api` now documents `ddcore.route`, `ddcore.setRoute`, the short route and
  the prefill (#57).
- `frm.addFieldButton` on a `Table`, `Report` or `HTML` field renders its button. The call was
  accepted and nothing appeared, because only a field with an input was given its buttons. On a
  `Table` or a `Report` field the button sits in the grid's toolbar, to the right; on an `HTML`
  field, under the content. A `Report` field shows it once the document is saved (#56).

## 0.23.3 — 2026-10-01

### Fixed

- Ctrl+S on a form saves the document with Caps Lock on. Windows reports the key in upper case
  then, the Desk missed it, and the browser opened its own "Save page as" dialog instead.

## 0.23.2 — 2026-10-01

### Added

- The Desk's two side bars collapse to a rail of icons, each with a button of its own. The
  navigation sidebar keeps the bell and To-Do icons with their counters as badges; the form's
  right column keeps one icon per section (assignments, shares, comments, history) with its count,
  and a click on any of them opens it again. The choice is kept per browser.
- `ddcore.json` takes an optional `title` that names the site — the sidebar's heading, the browser
  tab, the e-mails, `/health` — ahead of every app's title. Left out, the site is named by its own
  app as before. It changes the name only: `desk.home` and `desk.logo` stay with the apps. See
  `i18n` → "The site's name is the app's title" (#54).
- The three notifications the core writes itself — a document assigned, a document shared, a
  task due today — now also go out by email, in the recipient's language, with a button that
  opens the document. Each person turns each kind off on their profile ("Email notifications");
  the inbox notification is written either way. The choices are three opt-out fields on `User`
  (`mute_assignment_email`, `mute_share_email`, `mute_due_email`), reachable through
  `profile.updateMyProfile({emailNotifications: {assignment?, share?, due?}})`. Emails of an
  app's own notification rules are not affected by them. See `notifications` → "Email for the
  core's own notifications".
- `ddcore.siteUrl()` and `ddcore.docUrl(doctype, id)` in server code: the site's public address
  and the absolute desk address of a document, for a link in a mail template or a webhook
  payload. See `mail`.
- Mail template `core.notification` (`{title, message, doctype, id}`), the message behind the
  emails above.
- A comment on a form can be edited and deleted from the form's right column. Its author edits it
  in place and may delete it; a System Manager may delete anybody's. An edited comment says so
  next to its date, and `GET /api/comments/{doctype}/{id}` now returns `modified` so a client can
  tell.

### Changed

- **Users start receiving email on upgrade.** A site with a real mail transport
  (`DDCORE_MAIL_TRANSPORT=smtp` or `method`) mails assignees, share recipients and the owners of
  tasks due today from the first request after the upgrade; nothing was mailed for these before.
  Run `ddcore migrate` to add the three `User` columns. To keep a user silent, tick the three
  "No email when…" fields on their User record, or let them do it on their profile.
- A timeline entry — the `Comment` an assignment or a workflow transition writes in the user's
  name, any `comment_type` other than `Comment` — can no longer be edited or deleted by its owner
  through `PUT`/`DELETE /api/resource/Comment/{id}`; a System Manager still can. Server code that
  rewrote such an entry as an ordinary user now gets a `PermissionError`.

### Fixed

- A line break in a mail subject no longer reaches the `Subject` header: it is replaced by a
  space, so a subject built from text somebody typed cannot add headers to the message.

## 0.23.1 — 2026-10-01

### Fixed

- `doc.flags` set before `insert()`, `save()`, `submit()`, `cancel()` or `delete()` reaches the hooks
  of that write. Every hook used to get an empty `doc.flags` of its own, so server code could not
  tell `validate` that a write was the system's, and a note `validate` left was gone by `onUpdate`.
  The flags now travel with the write: the hooks share them, and what they set is back on the
  caller's `doc.flags` when the call returns. They stay server-side and per document, and their
  values must be JSON. See `controller-api` → "`doc.flags`: context for one write" (#55).

## 0.23.0 — 2026-10-01

### Breaking

- An app that imports a server file of another app by relative path now gets that app's own module
  instead of a private copy bundled into the importer: module state is shared, a test can replace
  an export the owner's controllers call, and the file's top-level code runs once. The importer has
  to declare the owner — a site where it does not refuses to load with `app b imports a module of
  app a: add "a" to requires in its ddcore.app.ts`. **Upgrade:** add the app to `requires` in
  `defineApp`; code that relied on its own copy of the other app's module state now shares it. See
  `conventions` → "Calling another app's server code" (#47).

### Fixed

- The site is named by its own app, not by a library that app `requires`: a required app loads
  first, so it used to take the Desk's title, `desk.home` and `desk.logo`. The apps nobody
  requires now come first for all three, and `/api/boot` carries the result as `site.home` and
  `site.logo` (#54).
- A `beforeEach`, `afterEach` or `beforeAll` written outside any `describe` applies to the tests of
  its own file. It used to run around every test of every file of the site, so a fake installed by
  one app's test file was in place during another app's tests. Tests run file by file as a result:
  a file's top-level tests, then its `describe` blocks (#48).

<!-- #endregion releases -->
