# Guidelines for AI agents

**ddcore** (*Data Driven Core*) — a Go + TypeScript + Postgres framework in the spirit of Frappe.
Read [`docs/agent/index.md`](docs/agent/index.md) (the API reference for writing apps) and
[`DEVELOPMENT.md`](DEVELOPMENT.md) (how work is done here, and the design's mental model).
`apps/testapp` is the fixture this checkout's `ddcore.json` loads, sized to what
`internal/acceptance` asserts and nothing more. The example
app is [ddcore-demo](https://github.com/jrvidotti/ddcore-demo); product apps, that one included,
live in their own repositories and build against the published binary.

- `make build` compiles the desk and the binary; `./bin/ddcore dev` serves `:8090` with hot reload; `make test` runs Go + TS.
- Dev Postgres: the `ddcore-pg` container on port 5455 via `make docker-up` (Docker Compose). The Go tests use the `ddcore_test` database (recreated).

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
- To restart explicitly when it really is necessary (after rebuilding the desk with
  `make build`, say), use `make stop` first.

---

## Releasing a version, and the app checkouts

Pushing a `v*` tag triggers the Release workflow: it publishes the cross-platform archives
(no container image: an app builds its own and fetches the binary). An app does not follow
the framework automatically — it pins the version it is written against in its own
`.ddcore-version`, which is what `install-ddcore.sh` downloads locally and what the app's
own Dockerfile fetches at build time.

So a version bump is not finished when the tag is pushed. After it:

1. **Update `.ddcore-version` in every app checkout** — `ddcore-demo` and the product apps,
   each in its own repository — then run `make test` there against the new binary and commit
   the pin. An app left behind is not broken, it is just still on the old release; an app
   whose pin moved without its tests being run is the actual risk.
2. **A breaking change does not reach the apps on its own.** Say so in the tag message and
   fix it in each app in the same pass, because nothing in the framework will: a removed
   fieldtype makes an app that still declares it fail to load, and `typegen.Write` never
   rewrites a `tsconfig.json` that already exists, so a change to the generated template
   only ever reaches apps created after it.
3. **SDK typings reach an app only through `ddcore types`**, run with the new binary. Until
   then the app typechecks against the SDK sources materialised under its `.ddcore/`.

---

## Essential references and practices

Always consult the documentation in [`docs/agent/index.md`](docs/agent/index.md):

1. **Synchronous TypeScript on the server:** app code running on the server (goja) is
   synchronous; never use `await` in a controller or a server-side service. Desk scripts using
   `@ddcore/desk-sdk` run in the browser and may be asynchronous.
2. **Generated typings:** never edit `.ddcore/types.d.ts` by hand. Use `./bin/ddcore types` or
   `make test`.
3. **English is the canonical language:** write every user-facing string — including a `label:`
   and a Select's values — in English, and put its translation in `translations/<lang>.csv`
   with `./bin/ddcore i18n extract`. A key without a translation fails `make check`, and would
   otherwise fail silently by rendering as English on a translated screen. See
   [`docs/agent/i18n.md`](docs/agent/i18n.md).
   **All code comments, documentation, scripts, Dockerfiles, and CI workflows in this repository must be written in English.**
4. **Tests:** always validate a change with `make test` (the Go tests plus the desk's and the
   apps' TypeScript).
5. **MCP:** `.mcp.json` points at `./bin/ddcore mcp`; use the ddcore MCP tools to inspect
   metadata, run methods and apply migrations rather than touching the database by hand.
