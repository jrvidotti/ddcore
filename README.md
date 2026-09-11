# ddcore

**ddcore** (*Data Driven Core*) is a port of the Frappe Framework model to **Go + TypeScript + PostgreSQL**,
built to be developed by agentic tools. One binary (`ddcore`) embeds esbuild and goja: apps are
TypeScript (DocTypes, rules, reports, screen scripts), run on the server without Node.
The desk (Svelte 5) is generated from the meta. Postgres is the only dependency.

The full design is in [`docs/plan-v1.md`](docs/plan-v1.md); the reference for agents is in
[`docs/agent/`](docs/agent/) (`ddcore docs`, or the MCP resources `ddcore://docs/*`).

*Em português: [`README.ptbr.md`](README.ptbr.md).*

## Developing the framework

```bash
make docker-up                               # dev Postgres (container ddcore-pg, port 5455)
make build                                   # desk (npm) + the binary at bin/ddcore
make migrate                                 # the core's DDL
./bin/ddcore user passwd Administrator admin # the Administrator's password
make dev                                     # http://localhost:8090  (Administrator / admin)
```

This checkout's `ddcore.json` loads `apps/demo` — a small projects-and-tasks app that doubles
as an executable tutorial and an end-to-end fixture (`./bin/ddcore demo` seeds the `DEMO`
project). Product apps live in their own repositories. In an outside project the script starts
with `ddcore init && ddcore new-app <name>`; `DDCORE_DSN` overrides the DSN of any command.

The development process (core vs. app, the working loop) is in
[`DEVELOPMENT.md`](DEVELOPMENT.md).

## Language

English is the source language. Every user-facing string — a `_()` call, a `label:`, a Select's
values — is written in English and is a catalogue key; translations live in
`translations/<lang>.csv` and are derived from the code by `ddcore i18n extract`. The contract
is in [`docs/agent/i18n.md`](docs/agent/i18n.md).

## Verification

```bash
make check   # desk typecheck + the translation catalogue
make test    # build + go vet + the Go and desk tests
```

The Go tests include `internal/acceptance`, which builds a temporary external app and checks
installation, boot, translation and `/app` against a real Postgres over real HTTP. The
disposable database named by `DDCORE_TEST_DSN` is recreated for each test.

## Layout

```
cmd/ddcore/        CLI
internal/         meta · db (migrate) · js (esbuild+goja) · engine (Document, permissions, jobs) · api · mcp · scaffold · typegen · i18nx (extractor)
core/             the embedded app: User, Role, Has Role, File, Comment, Version, Error Log, API Key
desk/             SvelteKit (its build is embedded in the binary)
packages/sdk      @ddcore/sdk — defineDoctype/defineController/… and the bridge's types
packages/desk-sdk @ddcore/desk-sdk — defineForm, frm.*, Dialog
docs/agent        the reference for agents
```

## MCP

`ddcore mcp` (stdio), or `http://localhost:8090/mcp` with `ddcore dev`. Tools: scaffold/migrate/types,
document CRUD, `call_method`, `sql_query`, `eval`, `run_tests`, `get_logs`. This repo ships a
`.mcp.json` for Claude Code.
