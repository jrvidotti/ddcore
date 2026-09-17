# CLI and the development loop

`ddcore.json` in the site directory: `dsn`, `apps` (directories), `port`, `workers`, `scheduler`, `lang`, `currency`, `currencyPrecision`, `rounding`, `timezone`, `dev`, `exportMaxRows`, `auth`, `ops`.
`currencyPrecision` defaults to the currency's ISO minor unit and `rounding` to `"commercial"`;
an unrecognised `rounding` stops the server at startup rather than quietly using another rule.
`DDCORE_DSN` overrides the dsn, and `DDCORE_DATA_DIR` the `dataDir`.
Every command accepts `--allow-older-binary`, the rollback override described in `backup`.
It is a global flag, stripped from the arguments before the command sees them: write it bare,
as `--allow-older-binary=true`, or set `DDCORE_ALLOW_OLDER_BINARY`. All three read the same
truths — `1`, `true`, `yes` or `on` — so `--allow-older-binary=false` leaves the override off.

Options may come **before or after** the positional arguments, as `--flag value` or
`--flag=value`; `--` ends the options and everything after it is positional. An undeclared
flag is an error (it never becomes an argument silently).

| Command | What it does |
|---|---|
| `ddcore init --dsn ... --port ...` | creates `ddcore.json`; if it exists, updates the dsn/port given (idempotent) |
| `ddcore new-app <name>` | scaffolds the app, registers it in ddcore.json, writes CLAUDE.md; it writes neither `version` nor a `ddcore` range, so add them by hand (see `conventions`) |
| `ddcore dev` | server with hot reload (rebuilds when a .ts/.csv is saved) and `--auto-migrate`; serves `/mcp` |
| `ddcore start` | production server (no watcher) |
| `ddcore migrate [--dry-run] [--prune]` | beforeSchema patches → DDL → afterInstall + fixtures → afterSchema patches → the drops → afterMigrate, in one transaction; then generates types. `--dry-run` reports the plan; a rename or conversion it cannot make safely is refused and nothing is applied (`migrations`) |
| `ddcore types` | generates `.ddcore/types.d.ts` and materialises the embedded SDK typings per app |
| `ddcore i18n extract [--app n\|--all] [--lang pt-BR] [--check] [--prune]` | rewrites `translations/<lang>.csv` from the code; `--check` reports and exits non-zero |
| `ddcore test [--app name] [--filter re] [-v]` | runs `*.test.ts` (each `it` in a rolled-back transaction) |
| `ddcore exec app.mod.fn --args '{}'` | runs a function as Administrator |
| `ddcore eval '<ts>' [--commit]` | runs loose TS with `ddcore.*` (rolls back by default) |
| `ddcore demo [--app name]` | runs `<app>.services.demo.generate` for every app that has `services/demo.ts` |
| `ddcore export <DocType>\|--all [--children] [--attachments] [--out DIR]` | exports the whole set to NDJSON/CSV with a manifest of checksums (see `export`) |
| `ddcore jobs list\|show\|stats\|retry\|cancel\|purge\|scheduled\|run <fn>\|work` | the queue and the scheduler (see `ops`); `show` is the only command that prints a job's arguments |
| `ddcore webhooks list\|replay <delivery>...` | outgoing webhook deliveries (see `webhooks`); a replay is recorded as an Audit Event |
| `ddcore audit list\|purge` | inspect and purge administrative audit events (see `audit`) |
| `ddcore user add <email> <name> --password x --role R` / `user passwd <email> <password>` | users; `passwd` also ends that user's other sessions |
| `ddcore user invite <email> <name> --role R` | creates the account with no password and sends the invitation link |
| `ddcore user reset <email>` | sends a password-recovery link |
| `ddcore user unlock <email>` | lifts a lockout without waiting out the window |
| `ddcore user sessions <email> [--revoke]` | lists, or ends, that user's sessions |
| `ddcore apikey <user> [--label x] [--days N]` | produces `key:secret` for `Authorization: token key:secret`; `--days` expires it |
| `ddcore mcp` | MCP server (stdio) |
| `ddcore docs [name]` | this documentation |
| `ddcore version` | prints `ddcore <version> (<os>/<arch>)`, the version the compatibility contract enforces; a binary built without `-ldflags` reports the default `0.1.0` and ranges are enforced against that (see `conventions`) |
| `ddcore maintenance on [--reason x]\|off\|status [--json]` | pauses HTTP writes, workers and the scheduler across every process; the CLI and MCP keep writing (see `backup`). Audited as `cli:$DDCORE_ACTOR`, else `$USER` or `$USERNAME` |
| `ddcore backup [--out f.tar] [--maintenance] [--no-files] [--to s3] [--keep N] [--json]` | one verifiable `.tar`: `pg_dump`, stored files, config, versions and checksums; secrets by name only; `--keep` prunes `DDCORE_BACKUP_DIR` and the bucket, never a custom `--out` (see `backup`) |
| `ddcore restore <f.tar\|s3:name> [--verify-only] [--smoke] [--smoke-user u] [--force] [--online] [--no-files] [--no-migrate] [--keep-sessions] [--json]` | verifies, restores into the configured database and storage, migrates, leaves the site paused, and times fetch, verify, database, files, migrate and smoke |
| `ddcore doctor [--json] [--strict] [--window N]` | probes the database, then reports meta, app versions and their `ddcore` ranges, pending DDL, undeclared columns and tables, pending patches, applied renames, queue, Error Log, scheduler, storage (the configured backend only — the bucket is never contacted) and single sign-on. Works with the database down. Exits non-zero on a critical finding; `--strict` also on a warning (see `ops`) |

`invite` and `reset` print the link instead of mailing it when no mail
transport is configured, which is what makes them usable in development and for
the first account on a new site. Once `DDCORE_MAIL_TRANSPORT` is set they print
only the expiry — a live recovery link has no business in shell history.

## An app in its own repository

```bash
mkdir my_app && cd my_app
ddcore init
ddcore new-app my_app --dir .
ddcore migrate
ddcore test
ddcore dev
```

Use `apps: ["."]` when the repository root is the app itself. The binary resolves the SDK
imports at run time and `ddcore types` writes the matching declarations under `.ddcore/`;
there is no need to keep the framework as a sibling checkout or to install the SDKs from
npm. `DDCORE_TEST_DSN` points at the disposable database the tests use.

## HTTP API

- `GET /healthz` (also `/api/health`) — liveness, never touches the database, always `200`.
- `GET /readyz` (also `/api/ready`) — readiness, `503` when the database does not answer. Both
  ignore a bad API key, so a stale monitoring token cannot report a healthy process dead.
- `GET /api/health/report` — the same picture with queue, Error Log and pool numbers; System Manager only.
- Every response carries `X-Request-Id`, and every error body repeats it as `requestId`. See `ops`.
- `POST /api/login {usr, pwd}` → `sid` cookie; a mutating request with a cookie needs the `X-DDCore-CSRF: 1` header.
- `GET /api/resource/<DocType>?filters=[...]&fields=[...]&order_by=&limit=&start=&with_count=1`
- `POST /api/resource/<DocType>` (insert), `GET/PUT/DELETE /api/resource/<DocType>/<name>`
- `POST /api/resource/<DocType>/<name>/<submit|cancel|amend|rename|method>`
- `POST /api/method/<app.folder.file.fn>` (whitelisted)
- `GET /api/meta/<DocType>`, `/api/boot`, `/api/search/link?doctype=&txt=`, `/api/search/global?txt=&limit=` (see `search`), `/api/report/<name>`, `/api/events` (SSE), `POST /api/upload`
- `GET /api/export/<DocType>?format=csv|ndjson&filters=[...]&children=1&attachments=1` — the whole filtered set as a download, gated by the `export` permission (see `export`)
- Errors: `{ "error": { "type", "title", "message", "key", "args" } }` with 417 (validation), 403, 404, 409, 401.
  `message` and `title` arrive already translated; `key` is the English template and `args` its values.
- `X-Lang` picks the language of a response. Without it the server uses `User.language`, then
  `Accept-Language` for an anonymous visitor, then `ddcore.json:lang`. `/api/meta` and
  `/api/boot` come back translated, and their ETag varies by language (`Vary: X-Lang`).

## MCP (`ddcore mcp`, or `http://localhost:<port>/mcp` in dev)

Tools: `list_doctypes`, `get_doctype`, `scaffold_doctype`, `validate_meta`, `migrate`, `i18n_extract`, `set_translations`, `generate_types`, `get_doc`, `list_docs`, `insert_doc`,
`update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`, `list_jobs`, `get_job`, `retry_job`, `cancel_job`, `purge_jobs`, `maintenance_status`, `maintenance_set`, `reload`, `list_apps`.
`/mcp` is exempt from the maintenance gate, so `maintenance_set` can switch the pause back
off; the writing tools still meet the engine's own guard while it is on (see `backup`).
Resources: `ddcore://docs/<name>`, `ddcore://meta/<DocType>`.
