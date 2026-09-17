# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

## Unreleased

### Added

- Single sign-on through OpenID Connect (SEC-05, partial): Google, PocketID or any OIDC provider,
  configured with `DDCORE_OIDC_*` in `.env`. It signs in existing Users only, linked by a verified
  e-mail address. `auth.passwordLogin: false` in `ddcore.json` leaves single sign-on as the only way
  in, except for Administrator. See [authentication](docs/agent/auth.md).
- Core/app compatibility contract (PRD-07): `defineApp({ ddcore: "<range>" })` declares the ddcore
  releases an app supports, and a binary outside the range refuses to load it. `ddcore doctor` and
  the export manifest report each app's version and range.

### Breaking

- An app whose `version` is not `MAJOR.MINOR.PATCH` (for example `"1.0"` is fine, `"beta"` is not)
  no longer loads. Fix the value or remove it.
