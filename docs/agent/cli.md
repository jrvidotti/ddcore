# CLI and the development loop

`ddcore.json` in the site directory: `dsn`, `apps` (directories), `ddcore` (the range of ddcore releases the site is tested against — see `conventions`), `port`, `workers`, `scheduler`, `title` (the site's name, when it should not be its app's title — see `i18n`), `lang`, `currency`, `currencyPrecision`, `rounding`, `timezone`, `exportMaxRows`, `importMaxRows`, `poolMaxConns`, `shutdownGraceSeconds`, `auth`, `ops`, `cors`.
`currencyPrecision` defaults to the currency's ISO minor unit and `rounding` to `"commercial"`;
an unrecognised `rounding` stops the server at startup rather than quietly using another rule.
`DDCORE_DSN` overrides the dsn, and `DDCORE_DATA_DIR` the `dataDir`.
`poolMaxConns` sizes the database pool requests run on, and `DDCORE_POOL_MAX_CONNS` overrides it;
unset, the DSN's `pool_max_conns` or pgx's default (the larger of 4 and the number of CPUs)
applies. A request holds its connection while it waits on a slow outbound call, so a site that
makes them may need it larger. The DSN may carry `pool_*` settings, which the cache invalidation
listener accepts as the pool does.
`workers` (default 2, `DDCORE_WORKERS` overrides it with a number) is how many job workers a
process runs: a number serves every queue, and an object such as `{ "default": 2, "bot": 2 }`
gives a queue a pool of its own — see `ops`, "Worker pools".
`shutdownGraceSeconds` (default 30, `DDCORE_SHUTDOWN_GRACE_SECONDS` overrides it) is how long a
process told to stop lets its running jobs finish before it interrupts them and gives them back to
the queue — see `ops`, "Stopping the process".
`cors.origins` lists the other origins whose pages may call the methods whitelisted with
`cors: true` (`"*"`, `scheme://host[:port]` or `scheme://*.domain`); `DDCORE_CORS_ORIGINS`,
comma-separated, replaces it. An entry no browser would send as an `Origin` stops the server at
startup. See `controller-api` → "Calls from another origin".
Every command accepts `--allow-older-binary`, the rollback override described in `backup`.
It is a global flag, stripped from the arguments before the command sees them: write it bare,
as `--allow-older-binary=true`, or set `DDCORE_ALLOW_OLDER_BINARY`. All three read the same
truths — `1`, `true`, `yes` or `on` — so `--allow-older-binary=false` leaves the override off.

Options may come **before or after** the positional arguments, as `--flag value` or
`--flag=value`; `--` ends the options and everything after it is positional. An undeclared
flag is an error (it never becomes an argument silently). `ddcore <command> -h` (or `--help`)
prints that command's options, or its subcommands for a command such as `import` or `jobs`.

| Command | What it does |
|---|---|
| `ddcore init [--name n] [--db-port p] [--dsn ...] [--port ...]` | creates `ddcore.json` (with a `ddcore` range for the running minor release), `.env.example`, `README.md`, `AGENTS.md` (with `CLAUDE.md` linked to it), `.mcp.json`, `.gitignore`, a `Dockerfile` on the official image pinned to the running release with its `.dockerignore`, and a `docker-compose.yml` running the Postgres of a local dsn (never overwrites an existing file). `--name`/`--db-port` build `postgres://n:n@localhost:p/n?sslmode=disable` (name defaults to the directory, port to 5432); `--dsn` is the alternative for an existing database. If `ddcore.json` exists, updates the dsn/port given (idempotent) |
| `ddcore new-app <name>` | scaffolds the app and registers it in ddcore.json; `ddcore.app.ts` gets `version: "0.1.0"`, and a `ddcore` range for the running minor release only when `ddcore.json` declares none (see `conventions`) |
| `ddcore dev` | server with hot reload (rebuilds when a .ts/.csv is saved) and `--auto-migrate`; serves `/mcp`; development mode (as `DDCORE_DEV=1` gives any other command) |
| `ddcore start` | production server (no watcher). Migrates first, like `migrate` — replicas take turns on an advisory lock; `--auto-migrate=false` or `DDCORE_AUTO_MIGRATE=0` leaves it to a separate step. `DDCORE_ADMIN_PASSWORD` gives a new site's Admin its password instead of a generated one |
| `ddcore deploy docker\|railway` | writes what a deployment builds from, leaving existing files alone: `Dockerfile` (official image, pinned to this release) and `.dockerignore`; `railway` adds `.railway/railway.ts` — the Railway project as Infrastructure as Code (service from the checkout's GitHub origin, PostgreSQL, `DATABASE_URL`, health check on `/api/ready`, secrets as `preserve()`) — and its `package.json`, applied with `railway config plan` / `apply` (see the deployment guide) |
| `ddcore migrate [--dry-run] [--prune]` | beforeSchema patches → DDL → afterInstall + fixtures → afterSchema patches → the drops → afterMigrate, in one transaction; then generates types. `--dry-run` reports the plan; a rename or conversion it cannot make safely is refused and nothing is applied (`migrations`) |
| `ddcore types` | generates `.ddcore/types.d.ts` and materialises the embedded SDK typings per app |
| `ddcore i18n extract [--app n\|--all] [--lang pt-BR] [--check] [--prune]` | rewrites `translations/<lang>.csv` from the code; `--check` reports and exits non-zero |
| `ddcore test [--app name] [--filter re] [-v]` | runs `*.test.ts` (each `it` in a rolled-back transaction) |
| `ddcore exec app.mod.fn --args '{}'` | runs a function as Admin |
| `ddcore eval '<ts>' [--commit]` | runs loose TS with `ddcore.*` (rolls back by default) |
| `ddcore demo [--app name]` | runs `<app>.services.demo.generate` for every app that has `services/demo.ts` |
| `ddcore export <DocType>\|--all [--children] [--attachments] [--out DIR]` | exports the whole set to NDJSON/CSV with a manifest of checksums (see `export`) |
| `ddcore import plan\|run\|status\|reconcile <dir> [--tenant <slug>]` | loads an export directory into this site, resumable and idempotent; `--tenant` loads into a tenant and scopes `status` and `reconcile` to it (see `import`) |
| `ddcore jobs list\|show\|stats\|retry\|cancel\|purge\|scheduled\|run <fn>\|work` | the queue and the scheduler (see `ops`); `show` is the only command that prints a job's arguments; `work --queue q[,q] [--workers N]` starts only those queues' pools |
| `ddcore webhooks list\|replay <delivery>...` | outgoing webhook deliveries (see `webhooks`); a replay is recorded as an Audit Event |
| `ddcore push keys [--subject mailto:…]` | prints a new VAPID key pair for `ddcore.push` as `.env` lines (see `push`); stores nothing |
| `ddcore audit list\|purge` | inspect and purge administrative audit events (see `audit`) |
| `ddcore user add <email> <name> --password x --role R` / `user passwd <email> <password>` | users; `passwd` also ends that user's other sessions |
| `ddcore user invite <email> <name> --role R` | creates the account with no password and sends the invitation link |
| `ddcore user reset <email>` | sends a password-recovery link |
| `ddcore user unlock <email>` | lifts a lockout without waiting out the window |
| `ddcore user sessions <email> [--revoke]` | lists, or ends, that user's sessions |
| `ddcore apikey <user> [--label x] [--days N]` | produces `key:secret` for `Authorization: token key:secret`; `--days` expires it. Recorded as an `apikey.create` audit event, as `ddcore.users.createApiKey` is |
| `ddcore tenant list\|create\|enable\|disable\|adopt` | the tenants of a site with `"tenancy": true`: `create <slug> [--title T] [--admin email]` invites its first System Manager, `disable` refuses its sign-ins and holds its jobs, `adopt <slug>` moves every row of the platform space into it, and `adopt <slug> --dry-run` lists what would move and every collision with the tenant, exiting non-zero on one (see `tenancy`) |
| `ddcore --tenant <slug> <command>` | runs a one-shot command inside a tenant (`eval`, `exec`, `user add`, `export`, `audit list`, …); the flag goes before the command. A server, a worker, `mcp` and `tenant` refuse it; `import` also takes it after the command |
| `ddcore mcp` | MCP server (stdio) |
| `ddcore docs [name]` | this documentation |
| `ddcore version` | prints `ddcore <version> (<os>/<arch>)`, the version the compatibility contract enforces; a binary built without `-ldflags` reports the default `0.1.0` and ranges are enforced against that (see `conventions`) |
| `ddcore maintenance on [--reason x]\|off\|status [--json]` | pauses HTTP writes, workers and the scheduler across every process; the CLI and MCP keep writing (see `backup`). Audited as `cli:$DDCORE_ACTOR`, else `$USER` or `$USERNAME` |
| `ddcore backup [--out f.tar] [--maintenance] [--no-files] [--to s3] [--keep N] [--json]` | one verifiable `.tar`: `pg_dump`, stored files, config, versions and checksums; secrets by name only; `--keep` prunes `DDCORE_BACKUP_DIR` and the bucket, never a custom `--out` (see `backup`) |
| `ddcore restore <f.tar\|s3:name> [--verify-only] [--smoke] [--smoke-user u] [--force] [--online] [--no-files] [--no-migrate] [--keep-sessions] [--json]` | verifies, restores into the configured database and storage, migrates, leaves the site paused, and times fetch, verify, database, files, migrate and smoke |
| `ddcore doctor [--json] [--strict] [--window N] [--no-update-check]` | probes the database, then reports meta, app versions and their `ddcore` ranges (and the site's, from `ddcore.json`), pending DDL, undeclared columns and tables, pending patches, applied renames, queue, Error Log, scheduler, storage (the configured backend only — the bucket is never contacted) and single sign-on. Also asks GitHub for the newest published release and warns when this binary is behind it; `--no-update-check` or `DDCORE_UPDATE_CHECK=off` skips that one lookup. Works with the database down. Exits non-zero on a critical finding; `--strict` also on a warning (see `ops`) |

`invite` and `reset` print the link instead of mailing it when no mail
transport is configured, which is what makes them usable in development and for
the first account on a new site. Once `DDCORE_MAIL_TRANSPORT` is set they print
only the expiry — a live recovery link has no business in shell history.

## An app in its own repository

```bash
mkdir my_app && cd my_app
ddcore init --name my_app          # add --db-port if 5432 is taken
docker compose up -d               # the Postgres init wrote docker-compose.yml for
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
- `GET /api/resource/<DocType>?filters=[...]&fields=[...]&order_by=&limit=&start=&with_count=1` (`limit` left out is 20; `limit=0` is every row; a negative or non-numeric one is a `ValidationError`)
- `POST /api/resource/<DocType>` (insert), `GET/PUT/DELETE /api/resource/<DocType>/<id>`
- `POST /api/resource/<DocType>/<id>/<submit|cancel|amend|rename|method>` (rename body: `{ "id": "<new id>" }`)
- `POST /api/method/<app.folder.file.fn>` (whitelisted); `OPTIONS` answers a CORS preflight for a method with `cors: true` (see `controller-api`)
- `GET /<prefix>/…` — an app's static site, when `www` in `defineApp` declares the prefix (see `www`)
- `GET /api/meta/<DocType>`, `/api/boot`, `/api/search/link?doctype=&txt=`, `/api/search/global?txt=&limit=` (see `search`), `/api/report/<name>`, `/api/events` (SSE), `POST /api/upload`
- `GET /api/export/<DocType>?format=csv|ndjson&filters=[...]&children=1&attachments=1` — the whole filtered set as a download, gated by the `export` permission (see `export`)
- `POST /api/data-import/<DocType>` (multipart: `file`, `mode=insert|update`, `dry_run=1`, `decimal`, `date_order`, `sep`, `columns`) and `GET /api/data-import/<DocType>/template?sep=` — CSV/XLSX rows as ordinary saves, gated by the `import` permission (see `data-import`)
- Errors: `{ "error": { "type", "title", "message", "key", "args" } }` with 417 (validation), 403, 404, 409, 401.
  `message` and `title` arrive already translated; `key` is the English template and `args` its values.
- `X-Lang` picks the language of a response. Without it the server uses `User.language`, then
  `Accept-Language` for an anonymous visitor, then `ddcore.json:lang`. `/api/meta` and
  `/api/boot` come back translated, and their ETag varies by language (`Vary: X-Lang`).

## MCP (`ddcore mcp`, or `http://localhost:<port>/mcp` in dev)

Tools: `list_doctypes`, `get_doctype`, `scaffold_doctype`, `extend_doctype`, `validate_meta`, `migrate`, `i18n_extract`, `set_translations`, `generate_types`, `get_doc`, `list_docs`, `insert_doc`,
`update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`, `list_jobs`, `get_job`, `retry_job`, `cancel_job`, `purge_jobs`, `maintenance_status`, `maintenance_set`, `reload`, `list_apps`.
The document tools (`get_doc`, `update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`) name the document with `id`.
`ddcore mcp` starts even when Postgres is down: `list_doctypes`, `get_doctype`, `scaffold_doctype`,
`extend_doctype`, `validate_meta`, `i18n_extract`, `set_translations`, `generate_types`, `reload`,
`list_apps`, `whats_new` and the resources work without it; every other tool reports the database
as unreachable and connects on the first call after it answers.
`/mcp` is exempt from the maintenance gate, so `maintenance_set` can switch the pause back
off; the writing tools still meet the engine's own guard while it is on (see `backup`).
Resources: `ddcore://docs/<name>`, `ddcore://meta/<DocType>`.
