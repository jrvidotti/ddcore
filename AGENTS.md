# Guidelines for AI agents

This repository follows the conventions and architecture documented in
[`CLAUDE.md`](CLAUDE.md), which is the primary reference for the ddcore framework, its
directory layout, its scripts and its code conventions.

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

## Essential references and practices

Always consult [`CLAUDE.md`](CLAUDE.md) and the documentation in
[`docs/agent/index.md`](docs/agent/index.md):

1. **Synchronous TypeScript on the server:** app code running on the server (goja) is
   synchronous; never use `await` in a controller or a server-side service. Desk scripts using
   `@ddcore/desk-sdk` run in the browser and may be asynchronous.
2. **Generated typings:** never edit `.ddcore/types.d.ts` by hand. Use `./bin/ddcore types` or
   `make test`.
3. **English is the source language:** write every user-facing string — including a `label:`
   and a Select's values — in English, and put its translation in `translations/<lang>.csv`
   with `./bin/ddcore i18n extract`. A key without a translation fails `make check`, and would
   otherwise fail silently by rendering as English on a translated screen. See
   [`docs/agent/i18n.md`](docs/agent/i18n.md).
4. **Tests:** always validate a change with `make test` (the Go tests plus the desk's and the
   apps' TypeScript).
5. **MCP:** use the ddcore MCP tools declared in `.mcp.json` to inspect metadata, run methods
   and apply migrations.
