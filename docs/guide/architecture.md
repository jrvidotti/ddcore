# Architecture & Mental Model

Understanding the architectural foundations and runtime execution model of `ddcore`.

---

## 1. Lineage & Core Philosophy

The design of `ddcore` (*Data Driven Core*) is inspired by the declarative model pioneered by the [Frappe Framework](https://frappeframework.com) (the foundation of ERPNext). 

In traditional application development, developers write repetitive code across multiple layers:
1. Database DDL statements (tables, columns, foreign keys, indexes).
2. ORM models or data transfer objects.
3. Serialization layers, validation logic, and REST controllers.
4. Administrative screens, forms, lists, and filtering widgets.
5. Permission gates and role checks on every endpoint.

Frappe introduced the concept of the **DocType** (*Document Type*): a single metadata specification describing an entity, its data types, relationships, permissions, and layout. From that declaration, the engine derives database schemas, CRUD APIs, and administrative UI screens automatically.

`ddcore` adopts this declarative DocType model, but re-engineers the runtime and execution environment with distinct technical foundations:
- **Go Host Runtime:** A single compiled binary embeds the HTTP server, asset bundler, and script runner.
- **Synchronous TypeScript on the Server:** Application logic is authored in TypeScript and executed synchronously inside embedded JavaScript runtimes.
- **PostgreSQL as Primary Engine:** Strict ACID guarantees, foreign keys, and relational schema migrations.
- **Reactivity via Svelte 5:** The Desk UI is built with Svelte 5 and embedded directly into the binary.
- **Native Model Context Protocol (MCP):** First-class support for AI development tools to inspect, scaffold, and test applications.

> **Compatibility Note:** `ddcore` does not provide binary or code-level compatibility with Frappe or Python apps. It shares the conceptual DocType paradigm while employing an entirely independent codebase and execution model.

---

## 2. The Runtime Architecture

The `ddcore` runtime is packaged as a single standalone binary (`ddcore`) that bundles:

```
┌────────────────────────────────────────────────────────┐
│                      ddcore Binary                     │
│                                                        │
│  ┌────────────────┐  ┌──────────────────────────────┐  │
│  │   Go HTTP      │  │        goja VM Pool          │  │
│  │   Router & API │  │  (Synchronous JS execution)  │  │
│  └───────┬────────┘  └──────────────┬───────────────┘  │
│          │                          │                  │
│  ┌───────▼──────────────────────────▼───────────────┐  │
│  │             ddcore Engine (Go)                   │  │
│  │  Meta · DDL Migration · Permlevels · Auth · Jobs │  │
│  └───────┬──────────────────────────┬───────────────┘  │
│          │                          │                  │
│  ┌───────▼────────┐  ┌──────────────▼───────────────┐  │
│  │  Svelte 5 Desk │  │          Native MCP          │  │
│  │ (Embedded SPA) │  │       (Stdio & HTTP)         │  │
│  └────────────────┘  └──────────────────────────────┘  │
└──────────────────────────────┬─────────────────────────┘
                               │ SQL (pgx)
                               ▼
                    ┌─────────────────────┐
                    │ PostgreSQL Database │
                    └─────────────────────┘
```

### Go Host Engine
The core engine is implemented in Go:
- **Routing & Networking:** High-throughput HTTP server routing API requests, authentication flows, and serving the embedded Desk SPA.
- **Database Layer:** Manages connection pooling, transaction lifecycles, and relational migrations using PostgreSQL (`pgx`).
- **Embedded desk:** SvelteKit frontend compiled and embedded using Go's `embed.FS`.

### Embedded Script Engine (`goja` + `esbuild`)
Application controllers and server-side services run inside [goja](https://github.com/dop251/goja), an ECMAScript 5.1+ runtime written in pure Go:
- **Transpilation:** TypeScript controllers are bundled and transpiled on the fly using embedded [esbuild](https://esbuild.github.io).
- **Synchronous Execution:** Server code is strictly synchronous. There is no event loop, and using `async`/`await` is neither supported nor required in controllers.
- **Isolation:** Each HTTP request or background job executes within an isolated Goja runtime instance, preventing cross-request variable leaks.

---

## 3. Transaction & Consistency Model

In `ddcore`, every data mutation runs inside an explicit, single PostgreSQL transaction:

```
Request Received
       │
       ▼
Begin Database Transaction
       │
       ▼
Document Lifecycle:
  1. beforeValidate(doc)
  2. Core validations (types, mandatory, uniqueKeys, link targets)
  3. validate(doc)
  4. beforeSave(doc)
  5. SQL INSERT or UPDATE
  6. afterInsert(doc) / onUpdate(doc)
       │
       ├─────────────────────────┐
    [Success]                 [Failure]
       │                         │
       ▼                         ▼
Commit Transaction         Rollback Transaction
(Changes persisted)        (Database remains unchanged)
```

### Key Principles:
1. **Atomic Rollback:** If any validation fails, or if controller code calls `ddcore.throw()`, the active transaction rolls back completely.
2. **No Application Commits:** Application code cannot manually trigger `commit()` or `rollback()`. The framework manages the transaction boundary.
3. **Rollback in Tests:** The `ddcore test` runner wraps each test case in a transaction that is rolled back upon completion, ensuring fast test execution and clean databases.
4. **External Side-Effects:** Operations that cannot be rolled back via SQL (such as sending emails or calling external HTTP webhooks) are recorded in transactional queue tables (`tab_mail_queue`, `tab_webhook_delivery`) and dispatched only **after** the transaction successfully commits.

---

## 4. The Desk (Client Architecture)

The administrative UI ("Desk") is an embedded single-page application built with **Svelte 5**:
- **Metadata-Driven Views:** Desk generates forms, lists, and standard filters dynamically by querying `/api/v1/meta/:doctype`.
- **Form Scripts (`.form.ts`):** Developers can customize client-side behavior (dynamic field visibility, calculated fields, custom buttons) using `@ddcore/desk-sdk`. Unlike server controllers, form scripts run in the browser and can be asynchronous.
- **No Production Node.js Server:** The Desk assets are compiled ahead of time into static HTML, JS, and CSS, and embedded directly inside the `ddcore` binary.

---

## 5. Agent-First Development & MCP

`ddcore` is built from the ground up to support AI-assisted and agentic development workflows:

- **Model Context Protocol (MCP):** The framework embeds a native MCP server (`ddcore mcp`).
- **Developer & Agent Parity:** Coding agents have access to the exact same capabilities as human developers: inspecting DocType schemas, creating scaffolding, applying migrations, running test suites, and viewing application logs.
- **Self-Documenting:** All core reference guides are embedded into the binary and available via MCP resources (`ddcore://docs/*`), providing instant context to AI assistants.

---

## Summary

`ddcore` pairs the developer productivity of metadata-driven models with the operational simplicity of a single compiled binary and PostgreSQL.

To learn how this architecture functions in practice, continue to the [Example App Walkthrough](/guide/demo).
