# Changelog

Changes that matter to an app built on ddcore, newest first. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/); versions follow the policy in
[`docs/agent/conventions.md`](docs/agent/conventions.md). Anything under **Breaking** carries the
upgrade path, and apps should keep their `ddcore:` range below that release until they have
taken it.

## Unreleased

### Added

- Backup, restore and maintenance (PRD-01, PRD-02):
  - `ddcore backup` writes one checksummed archive of the database, stored files, configuration
    and versions, with optional S3 upload and retention.
  - `ddcore restore` verifies the archive, restores it into an isolated target, migrates, and
    times each phase. `--smoke` adds a check pass.
  - `ddcore maintenance on|off` pauses writes and jobs on every process and shows a Desk banner.
  - `DDCORE_DATA_DIR` overrides `dataDir`. `ops.backupMaxAgeHours` warns on a stale backup.

- Global search (OPS-08): a Mod+K palette in the desk and `GET /api/search/global`, matching the
  title and search fields of every DocType the user can list, with roles, scopes, shares and field
  levels applied. `globalSearch` on a DocType opts it in or out. Core log DocTypes are opted out.
- Kanban and Gantt list views (OPS-08) through `defineListView({ kanban, gantt })`. Dragging a
  Kanban card saves its Select field.
- Single sign-on through OpenID Connect (SEC-05, partial): Google, PocketID or any OIDC provider,
  configured with `DDCORE_OIDC_*` in `.env`. It signs in existing Users only, linked by a verified
  e-mail address. `auth.passwordLogin: false` in `ddcore.json` leaves single sign-on as the only way
  in, except for Administrator. See [authentication](docs/agent/auth.md).
- Core/app compatibility contract (PRD-07): `defineApp({ ddcore: "<range>" })` declares the ddcore
  releases an app supports, and a binary outside the range refuses to load it. `ddcore doctor` and
  the export manifest report each app's version and range.

### Breaking

- A binary older than the one that last ran `migrate` on a database now refuses to open it,
  naming the core or app version that is older. Roll forward, or pass `--allow-older-binary`
  (`DDCORE_ALLOW_OLDER_BINARY=1`) when the migrations since were expand-only. Databases migrated
  before this release have no ledger row and are not checked until their next `migrate`.
- `storage.Store` has a new `List` method; an embedder with its own store must implement it.

- An app whose `version` is not a version — `"1"`, `"1.0"` and `"1.0.0"` are all fine, `"beta"` and
  `"1.0.0-rc.1"` are not — no longer loads. Fix the value or remove it.

### Fixed

- `^` and `~` with an abbreviated version now bound what the author left out, as npm does: `^0` is
  every `0.x`, `^0.0` every `0.0.x`, and `~1` every `1.x`. The spelt-out forms are unchanged.
- A job whose write is refused by maintenance mode goes back to the queue without consuming an
  attempt and without an Error Log row, instead of failing — on its last attempt it used to die of
  a pause that was nobody's fault.
- Global search applies its five-hits-per-DocType cap after ranking, so an exact match is no longer
  lost behind more recently modified rows, and `%` and `_` in the search text now match themselves
  instead of acting as wildcards.
- A `migrate` run by a build with no release version (a `dev` binary) no longer replaces the
  ledger's core version, which silently disarmed the rollback guard for every later binary.
- `ddcore.share.*` refuses a right it does not know (`override_scope` for `overrideScope`) with the
  same `417` as the endpoint, rather than dropping it and granting a share without it.
- Single sign-on clears the client address's failed attempts on a successful sign-in, as a password
  sign-in does, and a failure on the server's own side is reported as `sso_error=server` rather than
  blamed on the provider.
- `--allow-older-binary=true` is accepted; only the bare flag used to be.
- `ddcore doctor` reports a rollback refusal as its own critical instead of "the apps could not be
  loaded", and prints an S3 bucket with no prefix without a trailing slash.
- `ddcore restore` warns when it stops after the database was replaced: the target keeps the
  restored database and stays paused.
- A list view listed in `views` but never configured no longer shows a button that falls back to
  the table.
