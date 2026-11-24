# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

This file holds `Unreleased` and the current minor series. Each older series has its own file
under [`docs/changelog/`](docs/changelog/): [0.31](docs/changelog/0.31.md), [0.30](docs/changelog/0.30.md), [0.29](docs/changelog/0.29.md), [0.28](docs/changelog/0.28.md), [0.27](docs/changelog/0.27.md), [0.26](docs/changelog/0.26.md), [0.25](docs/changelog/0.25.md), [0.24](docs/changelog/0.24.md), [0.23](docs/changelog/0.23.md), [0.22](docs/changelog/0.22.md), [0.21](docs/changelog/0.21.md), [0.20](docs/changelog/0.20.md), [0.19](docs/changelog/0.19.md), [0.18](docs/changelog/0.18.md), [0.17](docs/changelog/0.17.md), [0.16](docs/changelog/0.16.md), [0.15](docs/changelog/0.15.md), [0.14](docs/changelog/0.14.md), [0.13](docs/changelog/0.13.md), [0.12](docs/changelog/0.12.md), [0.11](docs/changelog/0.11.md), [0.10](docs/changelog/0.10.md), [0.9](docs/changelog/0.9.md), [0.8](docs/changelog/0.8.md), [0.7](docs/changelog/0.7.md), [0.6](docs/changelog/0.6.md), [0.5](docs/changelog/0.5.md), [0.4](docs/changelog/0.4.md), [0.3](docs/changelog/0.3.md), [0.2](docs/changelog/0.2.md), [0.1](docs/changelog/0.1.md).
The binary serves them all: `ddcore://changelog` is this file, `ddcore://changelog/<minor>` an
older series, and `whats_new` reads across every one of them.

<!-- #region releases -->
## Unreleased

## 0.32.1 — 2026-11-24

### Added

- **Single sign-on can create the account on the first sign-in (#168).** `auth.sso.<id>.provision`
  (`{ "userType": "Website User", "roles": [] }`) makes a sign-in that would end in `no_account`
  create the User instead, for an address the provider vouches for and the domain list allows,
  and records `account.provision`. Admin, Guest, All and System Manager cannot be given this way,
  and a site with tenancy refuses the setting. Without it nothing changes. See `auth`,
  "Creating the account on the first sign-in".
- **A Website User lands on an app's static site after signing in, when that is where the
  sign-in asked to go (#168).** Before, anything but `/portal/…` became `/portal`; a `www` prefix
  is now honoured too, so a site served under `www` can be the product itself. See `www`,
  "Signed-in visitors".

## 0.32.0 — 2026-11-24

### Added

- **One person can work in several tenants, with one sign-in and roles per tenant (#167).** On a
  site with tenancy, a User row is now one tenant's membership of a person. The same e-mail added
  to a second tenant (`ddcore --tenant teste user add ana@example.com "Ana" --role …`, the desk, an
  invitation) joins that person instead of failing with "User already exists". Each tenant keeps
  its own roles, enabled flag, language and type for them. Password sign-in, single sign-on and
  `ddcore user link` all resolve to the one person. A session works in one of the person's own
  tenants, and they switch from the tenant menu in the user menu, which lists only their own
  tenants. `POST /api/tenant/enter` now answers them too, and refuses a tenant they do not work in
  exactly as one that does not exist. See `tenancy`, "People in several tenants".
- **`core.access_granted`** is the message a person gets when a tenant adds them while they already
  sign in elsewhere on the site. It carries no link to set a password. `users.invite` then answers
  `{ user, access: true }`.
- **`POST /api/impersonate` takes `tenant`**, to pick the tenant of a person who works in several
  when an operator impersonates them from the platform space.

### Changed

- **A sign-in lands in the tenant the person last worked in**, or else the first of their tenants
  by title. That includes operators who last entered a tenant.
- **A tenant's System Manager administers that tenant's row of a person, and nothing more.** For a
  person who also works elsewhere on the site, a tenant cannot set or reset the password, send an
  invitation or reset link, unlink an identity, disable them at the identity provider, change the
  address they sign in with, rename them, or lift their lockout. The refusal names no other
  tenant. A reset link such an administrator issued, and a password one of them chose, stop
  working once the person joins another tenant: the person then chooses their own with "I forgot my
  password". The person themself, from a password or single sign-on session, and the operator,
  still can. An API key or a credential provider's session is not the person speaking for their
  sign-in, and only another operator may impersonate an operator.
- **Disabling or deleting a User, or revoking their sessions, from inside a tenant affects that
  tenant only.** The password and the linked identities go with the person's last row.
- **API keys, sessions opened by an app's credential provider, and impersonations stay in the
  tenant they belong to.** Only an operator's platform key moves with `X-Tenant`.
- **`unique` fields of `User` hold per tenant**, like every other tenant DocType. `migrate` re-keys
  `tab_user` by `(tenant, id)`.
- **Account audit events are recorded in the space the action happened in.** They were recorded in
  the account's own tenant, which a person in several tenants no longer has.
- **Single sign-on does not map groups to roles for a person in more than one space**, and neither
  does the PocketID role sync. Each tenant gives those roles itself.
- **Passwords are the person's: they live in the site-wide `ddcore_person` table**, which tenant
  code cannot read, even through `ddcore.db.sql`. `ddcore.db.setValue("User", …, { new_password })`
  is refused. Save the User to set one.

### Breaking

- **`User.password_hash` is gone.** `migrate` copies every hash into `ddcore_person` and blanks the
  column, and `migrate --prune` drops it. App code that read the column gets nothing. To tell
  whether someone has a password, use `core.services.users.accountStatus` or the profile's
  `hasPassword`. A binary older than this release cannot sign anyone in on a migrated database:
  after a downgrade, set the passwords again with `ddcore user passwd`.
- **From the platform space, a person in several tenants needs the tenant named.** That covers
  `ddcore apikey`, `ddcore user reset`, `users.createApiKey`, impersonation, `ddcore.runAs`
  (inside `ddcore.tenant.run`) and `ddcore.test.asUser`. Pass `--tenant` on the command line,
  `tenant` to the endpoint, or enter the tenant first. People in one tenant are unaffected.

<!-- #endregion releases -->
