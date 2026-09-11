# CLI and the development loop

`ddcore.json` in the site directory: `dsn`, `apps` (directories), `port`, `workers`, `scheduler`, `site`, `lang`, `currency`, `currencyPrecision`, `rounding`, `timezone`, `dev`, `exportMaxRows`.
`currencyPrecision` defaults to the currency's ISO minor unit and `rounding` to `"commercial"`;
an unrecognised `rounding` stops the server at startup rather than quietly using another rule.
`DDCORE_DSN` overrides the dsn.

Options may come **before or after** the positional arguments, as `--flag value` or
`--flag=value`; `--` ends the options and everything after it is positional. An undeclared
flag is an error (it never becomes an argument silently).

| Command | What it does |
|---|---|
| `ddcore init --dsn ... --port ...` | creates `ddcore.json`; if it exists, updates the dsn/port given (idempotent) |
| `ddcore new-app <name>` | scaffolds the app, registers it in ddcore.json, writes CLAUDE.md |
| `ddcore dev` | server with hot reload (rebuilds when a .ts/.csv is saved) and `--auto-migrate`; serves `/mcp` |
| `ddcore start` | production server (no watcher) |
| `ddcore migrate [--dry-run] [--prune]` | beforeSchema patches → DDL → afterInstall + fixtures → afterSchema patches → the drops → afterMigrate, in one transaction; then generates types. `--dry-run` reports the plan; a rename or conversion it cannot make safely is refused and nothing is applied (`migrations`) |
| `ddcore types` | generates `.ddcore/types.d.ts` and materialises the embedded SDK typings per app |
| `ddcore i18n extract [--app n\|--all] [--lang pt-BR] [--check] [--prune]` | rewrites `translations/<lang>.csv` from the code; `--check` reports and exits non-zero |
| `ddcore test [--filter re] [-v]` | runs `*.test.ts` (each `it` in a rolled-back transaction) |
| `ddcore exec app.mod.fn --args '{}'` | runs a function as Administrator |
| `ddcore eval '<ts>' [--commit]` | runs loose TS with `ddcore.*` (rolls back by default) |
| `ddcore demo [--app name]` | runs `<app>.services.demo.generate` for every app that has `services/demo.ts` |
| `ddcore export <DocType>\|--all [--children] [--attachments] [--out DIR]` | exports the whole set to NDJSON/CSV with a manifest of checksums (see `export`) |
| `ddcore jobs list\|run <fn>\|work` | scheduler and queue |
| `ddcore user add <email> <name> --password x --role R` / `user passwd <email> <password>` | users |
| `ddcore apikey <user>` | produces `key:secret` for `Authorization: token key:secret` |
| `ddcore mcp` | MCP server (stdio) |
| `ddcore docs [name]` | this documentation |
| `ddcore doctor` | database, meta, pending DDL, undeclared columns and tables, pending patches, applied renames, scheduler |

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

- `POST /api/login {usr, pwd}` → `sid` cookie; a mutating request with a cookie needs the `X-DDCore-CSRF: 1` header.
- `GET /api/resource/<DocType>?filters=[...]&fields=[...]&order_by=&limit=&start=&with_count=1`
- `POST /api/resource/<DocType>` (insert), `GET/PUT/DELETE /api/resource/<DocType>/<name>`
- `POST /api/resource/<DocType>/<name>/<submit|cancel|amend|rename|method>`
- `POST /api/method/<app.folder.file.fn>` (whitelisted)
- `GET /api/meta/<DocType>`, `/api/boot`, `/api/search/link?doctype=&txt=`, `/api/report/<name>`, `/api/events` (SSE), `POST /api/upload`
- `GET /api/export/<DocType>?format=csv|ndjson&filters=[...]&children=1&attachments=1` — the whole filtered set as a download, gated by the `export` permission (see `export`)
- Errors: `{ "error": { "type", "title", "message", "key", "args" } }` with 417 (validation), 403, 404, 409, 401.
  `message` and `title` arrive already translated; `key` is the English template and `args` its values.
- `X-Lang` picks the language of a response. Without it the server uses `User.language`, then
  `Accept-Language` for an anonymous visitor, then `ddcore.json:lang`. `/api/meta` and
  `/api/boot` come back translated, and their ETag varies by language (`Vary: X-Lang`).

## MCP (`ddcore mcp`, or `http://localhost:<port>/mcp` in dev)

Tools: `list_doctypes`, `get_doctype`, `scaffold_doctype`, `validate_meta`, `migrate`, `generate_types`, `get_doc`, `list_docs`, `insert_doc`,
`update_doc`, `delete_doc`, `submit_doc`, `cancel_doc`, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`, `reload`, `list_apps`.
Resources: `ddcore://docs/<name>`, `ddcore://meta/<DocType>`.
