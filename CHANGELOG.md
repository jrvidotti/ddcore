# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

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

## 0.25.0 — 2026-10-03

### Added

- `ddcore.getDoc(doctype, id, { ignorePermissions: true })` loads a document the user has no
  role permission to read, as `insert` and `save` already allowed for writes, so a service can
  load, change and save a document under the caller's identity instead of switching to a
  system user. The user's access scopes and the tenancy wall still apply, and the document
  comes back unredacted: `ddcore.redact` it before returning it to a client.
  `doc.reload({ ignorePermissions: true })` reads it again the same way. See `scopes` (#69).
- `ddcore.users.createApiKey(user, { label?, days? })` returns `{ key, secret, expires }`: server
  code can issue an API key for another user, so provisioning a tenant with its users and its
  integration's key can be a single whitelisted method or `onTenantCreate` instead of a method
  followed by `ddcore apikey`. System Manager or Admin only; inside a tenant, only that tenant's
  users; from the platform space, the key is made in the user's own tenant. Each key is
  recorded as an `apikey.create` audit event on the User, without the secret, and so is every
  key `ddcore apikey` issues, which recorded none before. See `controller-api` (#72).
- `ddcore.db.savepoint(fn)` runs `fn` inside a savepoint. When `fn` throws, only its writes,
  messages and events are rolled back, the error is rethrown, and the transaction stays usable,
  so an app can catch a collision and carry on instead of failing on `current transaction is
  aborted`. `ddcore.db.setValue` and `doc.dbSet` now throw `DuplicateEntryError` for a value a
  unique index refuses, as `insert` and `save` do, so `e.name === "DuplicateEntryError"` tells
  an idempotent retry from a failure. See `controller-api` → "Savepoints" (#70).
- A whitelisted method declared with `pathTail: true` also answers below its own path:
  `/api/method/<path>/pix/1` reaches it, and `ctx.request.pathTail` holds `"pix/1"`,
  percent-decoded (`""` when the call names the method alone). A provider webhook that appends
  to the URL it was registered with, such as a bank posting to `<url>/pix`, can now land on the
  app directly. A method without the option still answers a sub-path with a 404. See *Inbound
  webhooks* in `controller-api` (#68).

### Changed

- `ddcore.db.lock(key)` is scoped to the current tenant on a site with tenancy, as cache keys
  and naming series already are: two tenants locking the same key no longer wait for each
  other. The platform space, and a site without tenancy, lock as before. An app that needs one
  lock across the whole site takes it from the platform space; a tenant prefix an app already
  writes into its keys keeps working, and can go (#73).

<!-- #endregion releases -->
