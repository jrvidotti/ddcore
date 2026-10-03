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
