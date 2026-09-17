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
- Core/app compatibility contract (PRD-07): `defineApp({ ddcore: "<range>" })` declares the ddcore
  releases an app supports, and a binary outside the range refuses to load it. `ddcore doctor` and
  the export manifest report each app's version and range.

### Breaking

- A binary older than the one that last ran `migrate` on a database now refuses to open it,
  naming the core or app version that is older. Roll forward, or pass `--allow-older-binary`
  (`DDCORE_ALLOW_OLDER_BINARY=1`) when the migrations since were expand-only. Databases migrated
  before this release have no ledger row and are not checked until their next `migrate`.
- `storage.Store` has a new `List` method; an embedder with its own store must implement it.

- An app whose `version` is not `MAJOR.MINOR.PATCH` (for example `"1.0"` is fine, `"beta"` is not)
  no longer loads. Fix the value or remove it.
