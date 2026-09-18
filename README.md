# ddcore

**ddcore** (*Data Driven Core*) is an open-source framework for building business applications
from TypeScript models and rules, with a Go runtime, PostgreSQL, a generated Desk, and built-in
MCP development tools.

Define DocTypes in TypeScript and get database tables, forms, lists, and REST endpoints. Keep
validation on the server, inside transactions. The `ddcore` binary embeds esbuild, the goja
JavaScript runtime and the Desk (Svelte 5), so your TypeScript runs on the server without a
Node.js process in production. The model follows ideas from the
[Frappe Framework](https://frappe.io/framework); it is not compatible with Frappe apps or ERPNext.

- **Documentation:** [ddcore.dev](https://ddcore.dev)
- **Live demo:** [demo.ddcore.dev](https://demo.ddcore.dev), source in
  [ddcore-demo](https://github.com/jrvidotti/ddcore-demo)
- **License:** [MIT](LICENSE)

This README has two paths. To write an application, you need only the published binary:
[Build an app](#build-an-app). To change ddcore itself: [Contribute to the framework](#contribute-to-the-framework).

---

## Build an app

You do not need to clone or compile this repository. An app lives in its own repository and
runs on the published binary.

### Prerequisites

- macOS or Linux (amd64 or arm64)
- [Docker](https://docs.docker.com/get-docker/) with Compose (recommended): `ddcore init` writes
  a `docker-compose.yml` that runs the database. Or PostgreSQL 14+ with an empty database, which
  you pass with `ddcore init --dsn "postgres://..."` instead.

### Install the CLI

```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
ddcore version
```

The script downloads the latest release from
[GitHub Releases](https://github.com/jrvidotti/ddcore/releases) into `/usr/local/bin` when that
is writable, otherwise `~/.local/bin`. Pin a version with `VERSION=v0.13.0` before `sh`.

### Create and run an app

```bash
mkdir my-project && cd my-project
ddcore init --name myapp --db-port 5432         # ddcore.json + docker-compose.yml
docker compose up -d                            # starts Postgres (myapp:myapp@localhost:5432/myapp)
ddcore new-app library                        # scaffolds apps/library and registers it
ddcore migrate                                # creates the tables, generates the typings
ddcore user passwd Administrator admin1234
ddcore dev                                    # hot reload; the port is in ddcore.json
```

`ddcore init` also writes the project's own `README.md`, an `AGENTS.md` for coding agents
(`CLAUDE.md` links to it) and a `.mcp.json` that registers `ddcore mcp`. Sign in as
`Administrator`. From there, the
[first-app tutorial](https://ddcore.dev/guide/first-app) walks through a DocType, a server-side
validation rule and a test.

### Where to go next

| You want to | Read |
| --- | --- |
| Learn the model: DocTypes, lifecycle, server vs. browser code | [Architecture](https://ddcore.dev/guide/architecture) |
| See a complete app: projects, tasks, reports, translations | [Demo walkthrough](https://ddcore.dev/guide/demo) · [ddcore-demo](https://github.com/jrvidotti/ddcore-demo) |
| Look up an API: controllers, fields, permissions, reports | [Reference](https://ddcore.dev/agent/) (also `ddcore docs`) |
| Deploy: binary, app files, PostgreSQL | [Deployment](https://ddcore.dev/guide/deployment) |

Three rules save most first-day surprises:

1. **Server TypeScript is synchronous.** Controllers and services run in goja, with no `await`
   and no Node APIs. Desk scripts (`@ddcore/desk-sdk`) run in the browser and may be async.
2. **Never edit `.ddcore/types.d.ts`.** `ddcore migrate` and `ddcore types` regenerate it.
3. **English strings are keys.** Write labels in English and translate with
   `ddcore i18n extract` ([i18n](https://ddcore.dev/agent/i18n)).

### Developing with an AI agent

`ddcore init` writes `AGENTS.md` with the project's conventions (`CLAUDE.md` links to it) and a
`.mcp.json` that points the agent at the MCP server. Claude Code picks both up when it opens the
folder. Other clients use the same command:

```json
{ "mcpServers": { "ddcore": { "command": "ddcore", "args": ["mcp"] } } }
```

The server gives the agent the embedded reference (`ddcore://docs/*`) and tools to inspect
metadata, scaffold DocTypes, preview and apply migrations, work with records, read logs and run
tests. The agent edits your TypeScript files with its own tools; the files stay the source of
truth. `ddcore dev` also serves MCP over HTTP at `/mcp`, behind an Administrator or System
Manager API key. MCP tools run with Administrator authority. They are development tooling:
`ddcore start` (production) does not mount them.

---

## Contribute to the framework

This section is for changing ddcore itself. It requires Go, Node.js (to build the Desk), Docker
and `make`.

```bash
git clone https://github.com/jrvidotti/ddcore && cd ddcore
make docker-up                                   # dev Postgres (container ddcore-pg, port 5455)
make build                                       # the Desk (npm) + the binary at bin/ddcore
make migrate                                     # the core's DDL
./bin/ddcore user passwd Administrator admin1234 # the Administrator's password
make dev                                         # http://localhost:8090
```

This checkout's `ddcore.json` loads `apps/testapp`, the fixture the Go acceptance suite needs. It
is kept deliberately thin (`./bin/ddcore demo` seeds the `DEMO` project). Example and product
apps belong in their own repositories, built against the published binary.

Read [`DEVELOPMENT.md`](DEVELOPMENT.md) before changing anything: it covers the design, the
split between core and app, and the working loop. [`ROADMAP.md`](ROADMAP.md) lists what is
planned and what is known to be missing. Agents working in this repository also follow
[`CLAUDE.md`](CLAUDE.md), and the checked-in `.mcp.json` points them at `./bin/ddcore mcp`.

### Verification

```bash
make check   # desk typecheck + the translation catalogue
make test    # build + go vet + the Go and desk tests
```

The Go tests include `internal/acceptance`, which builds a temporary external app and checks
installation, boot, translation and `/app` against a real Postgres over real HTTP. The
disposable database named by `DDCORE_TEST_DSN` is recreated for each test.

### Language

English is the source language. Every user-facing string (a `_()` call, a `label:`, a Select's
values) is written in English and is a catalogue key. Translations live in
`translations/<lang>.csv` and are derived from the code by `ddcore i18n extract`. The contract is
in [`docs/agent/i18n.md`](docs/agent/i18n.md). Code comments and documentation are in English.

### Layout

```
cmd/ddcore/        CLI
internal/          meta · db (migrate) · js (esbuild+goja) · engine (Document, permissions, jobs) · api · mcp · scaffold · typegen · i18nx (extractor)
core/              the embedded app: User, Role, Has Role, File, Comment, Version, Error Log, API Key
desk/              SvelteKit (its build is embedded in the binary)
packages/sdk       @ddcore/sdk: defineDoctype/defineController/… and the bridge's types
packages/desk-sdk  @ddcore/desk-sdk: defineForm, frm.*, Dialog
docs/agent         the API reference, embedded in the binary and published at ddcore.dev
docs/guide         the tutorials published at ddcore.dev
apps/testapp       the acceptance fixture
```
