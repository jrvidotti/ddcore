# Backup, restore, maintenance and rollback

Recovering a site takes four things, and ddcore ships one tool for each:

- **`ddcore backup`** writes the database, the stored files, the configuration and the
  versions into one verifiable archive.
- **`ddcore restore`** brings that archive back into an isolated instance, checks it and
  times each phase.
- **Maintenance mode** pauses writes and jobs for a controlled cutover.
- **The site version ledger** stops an older binary from opening a database a newer
  one migrated, unless you say the rollback is safe.

Secrets are never part of any of it. Provision them on every target separately.

## Maintenance mode

```
ddcore maintenance on [--reason "Upgrading to 2.0"]
ddcore maintenance status [--json]
ddcore maintenance off
```

The flag lives in the database (`ddcore_maintenance`), so every server and every
`jobs work` process sees it within two seconds. While it is on:

| Surface | Behaviour |
|---|---|
| HTTP | `POST`/`PUT`/`PATCH`/`DELETE` answer **503** with `{"error": {"type": "MaintenanceError", "extra": {"reason", "retryAfter"}}}` and a `Retry-After` header. `GET`/`HEAD`, sign-in, sign-out, `/api/auth/*`, link-title lookups and the probes stay open. |
| Engine | Insert, save, submit, cancel, delete, rename, `dbSet`/`setValue` and `enqueue` refuse with the same error. This also covers a `GET` whitelisted method that writes. Error Log rows are still written. A refusal never creates one. |
| Workers | Stop claiming jobs. A job that was already running finishes, but writes it tries are refused and it fails into its normal retry. |
| Scheduler | Skips every run (logged). A run it skips is not made up later. |
| Desk | Shows a banner with the reason (from `/api/boot`, the `maintenance` event, or the first refused save). |
| Readiness | **Unchanged.** `/readyz` stays 200: a paused site is a decision, and an orchestrator must not restart it out of one. `/api/health/report` includes `maintenance`, and `doctor` warns. |

**The CLI is the bypass.** Only the processes that serve traffic or run jobs (`dev`,
`start`, `jobs work`) enforce the flag. `backup`, `restore`, `migrate`, `exec`, `eval`,
`user` and the MCP server keep writing, and that window is exactly what they are for. A
migration always runs, including `dev --auto-migrate` inside a server. Turning the
flag on or off is recorded as the Audit Event `ops.maintenance_on` or
`ops.maintenance_off`. The MCP tools are `maintenance_status` and `maintenance_set`.

What it does not stop: raw `ddcore.db.sql` writes, session bookkeeping on sign-in,
and anything outside ddcore (a webhook receiver, a cron job talking to the database
directly).

## `ddcore backup`

```
ddcore backup [--out file.tar] [--maintenance] [--no-files] [--to s3] [--keep N] [--json]
```

Writes `<DDCORE_BACKUP_DIR>/ddcore-YYYYMMDD-HHMMSS.tar` (by default `<dataDir>/backups`).
The archive holds:

| Entry | Content |
|---|---|
| `db.dump` | `pg_dump --format=custom --no-owner --no-acl` of the whole database |
| `files/public/…`, `files/private/…` | every stored object, by storage key, from the `local` or `s3` backend |
| `config/ddcore.json` | the site's versioned configuration as it was |
| `config/env.json` | non-secret `DDCORE_*` values, plus the **names** of the secrets |
| `manifest.json` | core and app versions and ranges, the site version ledger's last row, applied patches, the last `ddcore_migration` id, PostgreSQL and `pg_dump` versions, the storage backend, row counts per `tab_*` table, file count and bytes, the secret names, and a sha256 and size for every other entry |

The dump, the row counts and the migration state come from one exported snapshot
(`pg_export_snapshot` + `pg_dump --snapshot`), so they describe the same instant even
on a live site. **Files are not part of that snapshot.** On a live site an upload
between the snapshot and the file copy can leave a File row without bytes, or bytes
without a row. `--maintenance` pauses the site for the run (and resumes it
afterwards, unless it was already paused), which makes files and database
consistent. Use it for a cutover. A nightly backup of a busy site usually doesn't.

The archive is written as `<out>.partial` and renamed only once complete, so a failed
run leaves nothing that looks like a backup. Each run is recorded in
`ddcore_backup_log` (what `doctor` reads) and as an Audit Event `backup.create`
(`Denied` when it failed, with the redacted error).

**Requirements.** `pg_dump` must be on `PATH` and at least as new as the server (major
version), which the command checks before dumping. Container images need
`postgresql-client-<server major>` or newer. The connection's password goes to the
child process in `PGPASSWORD`, never in its argument list.

**Off-site copy.** `--to s3` uploads the finished archive to
`s3://<bucket>/<prefix>/ddcore-….tar`. Every `DDCORE_BACKUP_S3_*` variable you leave
unset falls back to the matching `DDCORE_S3_*`, and the prefix defaults to
`<DDCORE_S3_PREFIX>/backups`, so a site already storing files in a bucket needs no
more configuration. Use a separate bucket (or at least separate credentials with
write-only access) when a compromised application server must not be able to delete
its own backups. `--keep N` (or `DDCORE_BACKUP_KEEP`) then deletes all but the newest
N archives, locally and in the bucket. It only considers names this command
generates.

**Scheduling.** The binary does not schedule backups. Run the command from cron, a
systemd timer, a Kubernetes CronJob or Railway's cron service. Set
`ops.backupMaxAgeHours` in `ddcore.json` to make `doctor` and the health report warn
when the newest successful backup is older than that. It is off by default, because a
site backed up by its platform's own snapshots leaves no row to check.

```json
{ "ops": { "backupMaxAgeHours": 26 } }
```

**Encryption.** The archive is not encrypted. It contains the whole database,
including password hashes and `ddcore_vault` ciphertexts. Store it in an encrypted
bucket or wrap it (`age`, `gpg`) before it leaves the machine.

## `ddcore restore`

```
ddcore restore <archive.tar | s3:<name>> [--verify-only] [--smoke] [--smoke-user u]
               [--force] [--no-files] [--no-migrate] [--keep-sessions] [--online] [--json]
```

It restores into whatever this directory is configured for: `DDCORE_DSN`,
`DDCORE_DATA_DIR`/`DDCORE_STORAGE`/`DDCORE_S3_*`. A restore drill beside production
is therefore a second `.env` pointing at a new database and a new data directory or
bucket prefix. The command runs these phases in order:

1. **fetch**: a local path, or `s3:<name>` downloaded from the backup bucket.
2. **verify**: the whole archive is unpacked and every entry is checked against the
   manifest (size and sha256). The command refuses an entry the manifest doesn't
   list, a listed entry that is missing, any path outside the backup layout, and an
   unknown archive format. `--verify-only` stops here.
3. **target checks**: the command refuses an archive made by a newer core unless
   `--allow-older-binary`. It refuses a database that already holds a site unless
   `--force`. With `--force`, it switches maintenance on in the target first and
   waits for its servers to pause.
4. **database**: `pg_restore --clean --if-exists --no-owner --no-acl --exit-on-error
   --single-transaction`. A failure leaves the target as it was.
5. **files**: every archived object is written to the target store. Objects already
   there and absent from the archive are left alone.
6. **state**: maintenance is left **on** (unless `--online`) and every session is
   deleted (unless `--keep-sessions`), so nothing acts on restored data and nobody is
   signed in to it until someone has looked.
7. **migrate**: the engine loads, the version ledger check runs, then `migrate`
   brings the schema up to this binary (unless `--no-migrate`). Restoring an old
   archive with a new binary is how you upgrade it.
8. **smoke** (`--smoke`): readiness, then row counts per table (fewer rows than
   archived is a failure, more is allowed for fixtures), then up to 20 random
   `File` rows whose bytes must open. It then signs in as `--smoke-user` with
   `DDCORE_SMOKE_PASSWORD` (the session it creates is deleted), or without that
   user checks that an enabled administrator exists. A failed check exits non-zero.

The output lists each phase's duration and the total, which is the measured recovery
time. The restore is recorded as the Audit Event `backup.restore`.

### A restore drill

```bash
# .env.drill — never the production database or bucket
DDCORE_DSN=postgres://ddcore:…@db/ddcore_drill
DDCORE_DATA_DIR=/srv/drill
DDCORE_WEBHOOKS=off              # no outgoing business effects from restored data
DDCORE_MAIL_TRANSPORT=log
DDCORE_SECRET_KEY=…              # the production vault key, provisioned separately

createdb ddcore_drill
env $(cat .env.drill | xargs) DDCORE_SMOKE_PASSWORD=… \
  ddcore restore s3:ddcore-20260917-020000.tar --smoke --smoke-user ops@example.com
```

Then run the app's critical flow by hand, or with `ddcore exec` or an app test,
against the drill site while it is still in maintenance. Record the total time
against the agreed recovery time objective (RTO). The recovery point objective (RPO)
is the age of the archive: the time between backups is the data you agree to lose.
Drop the drill database afterwards.

## Rollback and the site version ledger

Every `migrate` appends a row to `ddcore_site_version` whenever the core or an app
version changed since the last one. The row holds the binary's core version and
each app's version. On startup (and on every command that loads the engine) the
binary compares itself with the newest row:

- same or newer: proceeds;
- **older core or older app**: refuses, naming what is older, for example
  `core 0.14.0 (database migrated by 0.15.0)`;
- a `dev`/`latest` build, or an app without a version, cannot be compared and is
  not refused.

`--allow-older-binary` (any command) or `DDCORE_ALLOW_OLDER_BINARY=1` overrides the
refusal. This is the deliberate rollback, and it is safe only when every migration
since the older release was **expand-only**, meaning it added nothing the old code
cannot ignore. See the expand → backfill → contract route in
[migrations](migrations.md). An old binary running `migrate` with the override
records its own, older versions as the newest row. A contraction (a dropped or
renamed column, a converted type) is never rollback-compatible. Its only way back is
a restore.

### Cutover and return

1. `ddcore maintenance on --reason "…"` on the old site. Wait for in-flight jobs
   (`ddcore jobs stats`).
2. `ddcore backup --maintenance --to s3`. This is the return point.
3. Deploy the new release, `ddcore migrate`, run the smoke flow while still paused,
   then `ddcore maintenance off`.
4. **Return**, if the release must be withdrawn:
   - *Before reopening:* redeploy the old binary. If the migration was expand-only,
     start it with `--allow-older-binary`. Otherwise `ddcore restore <archive> --force`
     with the old binary.
   - *After reopening:* writes made on the new release are **not** carried back.
     Restoring the archive discards them, and nothing replays them. Keep exactly one
     system taking operational writes at any time. Either roll forward with a fix, or
     accept the loss and export what was written after the cutover (`ddcore export`,
     filtered by `modified`) for a manual re-entry.

## Configuration

| Variable | Meaning |
|---|---|
| `DDCORE_DATA_DIR` | overrides `dataDir` (uploads, local backups) |
| `DDCORE_BACKUP_DIR` | local archive directory, default `<dataDir>/backups` |
| `DDCORE_BACKUP_KEEP` | default for `--keep`, 0 keeps all |
| `DDCORE_BACKUP_S3_ENDPOINT`, `_REGION`, `_BUCKET`, `_ACCESS_KEY`, `_SECRET_KEY`, `_PREFIX`, `_USE_SSL`, `_PATH_STYLE` | the backup bucket; each defaults to its `DDCORE_S3_*` counterpart |
| `DDCORE_ALLOW_OLDER_BINARY` | `1` = the rollback override |
| `DDCORE_SMOKE_PASSWORD` | the password `restore --smoke --smoke-user` signs in with |
| `ops.backupMaxAgeHours` (`ddcore.json`) | warn when the newest backup is older; 0 = off |

## Not covered

- Point-in-time recovery (WAL archiving). The recovery point is the last archive.
  Use your PostgreSQL provider's PITR for anything tighter.
- Archive encryption, and in-binary scheduling (see above).
- Replaying writes made after a cutover onto a restored or rolled-back site.
- A restore with `--force` drops and recreates the archived objects. It does not
  remove tables the target has and the archive does not, nor stored files absent
  from the archive.
- `pg_dump`/`pg_restore` are external dependencies whose major version must keep
  up with the server.
