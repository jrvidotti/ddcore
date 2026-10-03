# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

### Fixed

- Inside a tenant, a shared DocType no longer looks editable only to fail at save: the
  permissions the desk receives for it leave out write, create, delete, submit, cancel, amend
  and import, so Save, New, Delete, Rename, Duplicate and Import disappear, and its form and
  list say "Shared by every tenant: read only here" (an operator also gets **Go to the
  platform**, back to the same page). See `tenancy` (#75).
- A saved document's form is read only when the user may not write it, for any DocType, not
  only a Single or one under a workflow; its fields were left editable before (#75).

## 0.26.1 — 2026-10-03

### Added

- A field can stay out of the desk's **Duplicate** with `noCopy: true`; the copy takes the
  field's default, and a `Table` with it starts empty. See `fieldtypes` (#74).
- On a site with tenancy, desk URLs name their tenant (`?tenant=<id>`), so a link copied from
  the address bar opens in the space it was copied from. A link naming another tenant no
  longer loads the page: an operator is asked to enter that tenant (which moves the session,
  every tab with it), and a tenant's own user is told the link is not theirs. See `tenancy`.

### Changed

- **Duplicate** no longer copies a `unique` field nor a `readOnly` one (a `fetchFrom` still
  follows its Link), on the document and on its child rows: the copy takes their defaults,
  so it no longer fails at save with a duplicate value or carries what the server wrote on
  the original (#74).

## 0.26.0 — 2026-10-03

### Added

- An app can serve a static site to anyone under a URL prefix of its own:
  `defineApp({ www: { "/r": "client/checkout/build" } })` serves that folder at `/r/`, with
  `index.html` for a directory and as the fallback of a client-side route (`fallback: null`
  for a plain 404), so a public page — a SvelteKit `adapter-static` build with
  `paths.base: "/r"` — no longer needs a server of its own. `GET`/`HEAD` only, `/r` redirects
  to `/r/`, a path that climbs out of the folder is a 404, and every file goes out with
  `nosniff`, a referrer policy and `X-Frame-Options: DENY` (`frame: true` lifts it). A prefix
  is one lowercase segment, not one ddcore uses nor another app's; the load says which rule a
  declaration broke. The site's data comes from guest methods on the same origin. See `www`
  (#68).
- A whitelisted method declared with `cors: true` can be called by pages on the origins listed
  in `"cors": { "origins": [...] }` in `ddcore.json`, or `DDCORE_CORS_ORIGINS` (comma-separated):
  `https://shop.example.com`, `https://*.partner.example` for its subdomains, or `*`. The
  preflight answers `204` with the origin echoed and the method's own `methods`, and the call's
  response (an error too) is readable by the page, `X-Request-Id` included. Credentials are
  never allowed, so the caller sends an API key or comes as Guest. Every other route, and every
  method without `cors`, answers as before. See `controller-api` → "Calls from another origin"
  (#68).

<!-- #endregion releases -->
