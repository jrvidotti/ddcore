# Guidelines for AI agents

**ddcore** (_Data Driven Core_) — a Go + TypeScript + Postgres framework in the spirit of Frappe.
Read [`docs/agent/index.md`](docs/agent/index.md) (the API reference for writing apps) and
[`DEVELOPMENT.md`](DEVELOPMENT.md) (how work is done here, and the design's mental model).
`apps/testapp` is the fixture this checkout's `ddcore.json` loads, sized to what
`internal/acceptance` asserts and nothing more. The example
app is [ddcore-demo](https://github.com/jrvidotti/ddcore-demo); product apps, that one included,
live in their own repositories and build against the published binary.

- `make build` compiles the desk and the binary; `./bin/ddcore dev` serves `:8090` with hot reload; `make test` runs the whole suite (Go + TS) — see "Tests" below before running it.
- Dev Postgres: the `ddcore-pg` container on port 5455 via `make docker-up` (Docker Compose). The Go tests use databases named `ddcore_test_p<pid>…`, one set per run, cloned from a migrated template (`internal/testdb`).

---

## The development server (`dev`)

- **Do not start a development server (`./bin/ddcore dev` or `make dev`) if one is already running.**
- Before trying to start it, always check whether port 8090 is taken or answering:
  ```bash
  lsof -ti tcp:8090
  # or
  curl -I http://localhost:8090
  ```
- The development server has **automatic hot reload** (file watching through `watch.Apps`).
  Editing a doctype, a controller, a service or a translation CSV reloads the definitions in
  memory, so **restarting the process is not necessary**.
- **Always run `make build` when you finish changes on the current branch, and after merging
  anything into it.** Hot reload covers app files only; the desk and the binary that
  `./bin/ddcore dev` serves are rebuilt by `make build`, so skipping it leaves the running
  server on stale code.
- To restart explicitly when it really is necessary (after rebuilding the desk with
  `make build`, say), use `make stop` first.

---

## Essential references and practices

Always consult the documentation in [`docs/agent/index.md`](docs/agent/index.md):

1. **Synchronous TypeScript on the server:** app code running on the server (goja) is
   synchronous; never use `await` in a controller or a server-side service. Desk scripts using
   `@ddcore/desk-sdk` run in the browser and may be asynchronous.
2. **Generated typings:** never edit `.ddcore/types.d.ts` by hand. Use `./bin/ddcore types`.
3. **English is the canonical language:** write every user-facing string — including a `label:`
   and a Select's values — in English, and put its translation in `translations/<lang>.csv`
   with `./bin/ddcore i18n extract`. A key without a translation fails `make check`, and would
   otherwise fail silently by rendering as English on a translated screen. See
   [`docs/agent/i18n.md`](docs/agent/i18n.md).
   **All code comments, documentation, scripts, Dockerfiles, and CI workflows in this repository must be written in English.**
4. **Tests: run the tests of what you changed, in isolation — not `make test`.** The full suite
   takes minutes and loads the shared Postgres, so **never run `make test` on your own
   initiative**; run it only when the user asks, or before a release or a merge that touches
   many packages. Pick the narrowest command that covers the change:
   ```bash
   go test ./internal/meta/                              # one Go package
   go test ./internal/engine/ -run 'TestComputed|TestGrid' # the tests you touched, by name
   cd desk && npx vitest run src/lib/grid-rows.test.ts   # one desk test file
   cd desk && npm run check                              # desk types, after a .svelte/.ts change
   ./bin/ddcore test --app testapp                       # the fixture app's TS tests
   ./bin/ddcore i18n extract --all --lang pt-BR --check  # translations, after adding a string
   ```
   `internal/engine`, `internal/api` and `internal/acceptance` are the slow packages: always
   narrow them with `-run`. Add `-count=1` to skip Go's test cache. Never run two full suites at
   once, and say in your report which tests you ran and which you did not.
5. **MCP:** `.mcp.json` points at `./bin/ddcore mcp`; use the ddcore MCP tools to inspect
   metadata, run methods and apply migrations rather than touching the database by hand.
   `ddcore://changelog` and the `whats_new` tool report what changed since the running version.
6. **Releases: feed the changelog before the tag.** Every change an app would notice goes under
   `## Unreleased` in [`CHANGELOG.md`](CHANGELOG.md) **in the same commit that makes it** —
   under `Added`, `Changed`, `Fixed`, or `Breaking` with its upgrade path.
   **Before tagging a version, move the `Unreleased` entries under a new
   `## <version> — <YYYY-MM-DD>` heading and leave `Unreleased` empty; only then create and push
   the `v*` tag**, which is what triggers the release workflow. Never tag first: the tag is what
   publishes the binaries, and a release whose changelog section does not exist ships news
   nobody can read — the binary serves `CHANGELOG.md` over MCP and `ddcore doctor` points at it
   when a newer release exists. Which digit to bump is in
   [`docs/agent/conventions.md`](docs/agent/conventions.md); the full procedure is in
   [`DEVELOPMENT.md`](DEVELOPMENT.md).
