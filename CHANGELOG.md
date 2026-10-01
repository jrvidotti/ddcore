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
