---
layout: home

hero:
  name: ddcore
  text: Data Driven Core
  tagline: Build business applications from TypeScript models. Define your data dictionary once; ddcore drives database tables, forms, lists, APIs, and access controls automatically.
  actions:
    - theme: brand
      text: Build your first app
      link: /guide/first-app
    - theme: alt
      text: Try live demo
      link: https://ddcore-demo.up.railway.app

features:
  - title: One Model, Several Layers
    details: DocTypes drive database tables, standard forms, lists, REST endpoints, and generated TypeScript interfaces automatically.
  - title: Server-Side Business Rules
    details: Synchronous TypeScript with transactional writes, commit on success, and rollback on failure. Tests run in rolled-back transactions.
  - title: Generated Svelte 5 Desk
    details: A modern administrative interface embedded directly in the single binary, with forms, lists, reports, and configurable workspaces.
  - title: Built-in MCP Tools for Agents
    details: Native Model Context Protocol server lets AI coding agents inspect metadata, scaffold, migrate, query, and test applications.
  - title: Business Application Primitives
    details: Authentication, field permissions (permlevel), user scopes, approval workflows, signed webhooks, encrypted vault, and i18n.
  - title: Single Binary & Simple Runtime
    details: The compiled Go binary embeds esbuild, goja, and the Desk. PostgreSQL is the only required external service in production.
---

<div class="tip custom-block" style="padding-top: 8px">

**MIT-licensed · Data Driven Core (Go + TypeScript + PostgreSQL) · Self-hosted**

</div>

## Quick Install (CLI)

Install the standalone `ddcore` command-line tool on macOS or Linux:

```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
```

> **Note:** This command installs the `ddcore` CLI binary to `~/.local/bin` (or `/usr/local/bin`). To create an application and connect it to PostgreSQL, follow the step-by-step [Build Your First App](/guide/first-app) guide.

---

## From a Model to a Working Application

In `ddcore`, you define business entities as **DocTypes** in TypeScript. The engine takes care of the schema, validation, REST endpoints, and UI:

```typescript
// doctypes/task/task.doctype.ts
import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Task",
  module: "Projects",
  label: "Task",
  naming: { field: "code" },
  titleField: "title",
  trackChanges: true,
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true, inListView: true },
    { fieldname: "project", fieldtype: "Link", label: "Project", options: "Project", reqd: true, inListView: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "priority", fieldtype: "Select", label: "Priority", options: ["Low", "Medium", "High"], default: "Medium" },
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Open", "In progress", "Completed"], default: "Open" },
    { fieldname: "due_date", fieldtype: "Date", label: "Due date", reqd: true, inListView: true },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, delete: true },
    { role: "Project Contributor", read: true, write: true, create: true },
  ],
});
```

Server-side business rules are written synchronously in controllers:

```typescript
// doctypes/task/task.controller.ts
import { defineController, _ } from "@ddcore/sdk";
import type { Task } from "../../.ddcore/types";

export default defineController<Task>("Task", {
  validate(doc) {
    const projectStart = ddcore.db.getValue<string>("Project", doc.project!, "start_date");
    if (projectStart && doc.due_date && doc.due_date < projectStart) {
      ddcore.throw(_("The due date cannot be earlier than the project start ({0}).", [projectStart]), {
        title: _("Invalid due date"),
      });
    }
  },
});
```

Once defined, `ddcore migrate` automatically creates the PostgreSQL table `tab_task`, indexes, and generated TypeScript types. Running `ddcore dev` starts the application with automatic hot reload and an interactive Desk at `http://localhost:8090`.

Test the [live demo](https://ddcore-demo.up.railway.app), explore the complete reference implementation in the [ddcore-demo](https://github.com/jrvidotti/ddcore-demo) repository, and read the [Example App Walkthrough](/guide/demo).

::: tip Why "Data Driven Core"?
In **ddcore**, your data dictionary (DocType) is the single source of truth that drives all application layers: PostgreSQL relational schemas, REST APIs, reactive Svelte 5 Desk screens, strict TypeScript types, and permission rules.
:::

---

## Core Capabilities

### 1. One Model, Several Application Layers
A single DocType declaration creates:
- A PostgreSQL table with exact column types, foreign keys, and indexes.
- Standard REST API endpoints (`/api/v1/document/:doctype`) with field-level permissions.
- Generated TypeScript types under `.ddcore/types.d.ts` for strict typing across controllers and desk scripts.
- Rich form and list interfaces in the Desk.

### 2. Server-side Business Rules
Controllers run inside an embedded JavaScript engine (goja) synchronously without `async`/`await`. Each mutation runs inside a single PostgreSQL transaction:
- On success, changes are committed atomically.
- On error (or `ddcore.throw()`), the database transaction is cleanly rolled back.
- Integration tests run inside rolled-back transactions for fast, repeatable verification.

### 3. Generated Administrative Desk
The user interface is powered by Svelte 5 and embedded directly inside the Go binary:
- Dynamic forms with client-side triggers, validations, and custom scripts (`defineForm`).
- Searchable list views with standard filters, sorting, and pagination.
- Role-based workspaces, configurable KPI cards, and embedded charts (`defineWorkspace`, `defineReport`).

### 4. Built-in MCP Development Tools
ddcore includes a native Model Context Protocol (MCP) server accessible via stdio (`ddcore mcp`) or HTTP during development:
- AI coding agents can inspect DocType schemas, scaffold new apps and models, execute database migrations, run unit tests, and query data.
- Developers and agents use the exact same declarative contracts.

### 5. Essential Business Primitives Included
Common enterprise requirements are built into the framework rather than reimplemented per application:
- **Authentication & Security:** Argon2id password hashing, session tokens, lockout rules, and permission levels (`permlevel`).
- **User Permission Scopes:** Row-level data isolation restricting users to specific companies, branches, or territories.
- **Approval Workflows:** Declarative multi-state workflows with atomic state transitions and action buttons.
- **Encrypted Vault:** Secure credential storage with AES-GCM encryption.
- **Audit Logging & Webhooks:** Tamper-evident administrative audit logs and HMAC-signed webhook delivery.

### 6. Predictable Operations & Deployment
Running ddcore in production requires just the single binary, your application folder, and a PostgreSQL database. No Node.js runtime, build pipelines, or container orchestrators are required for base deployments.

---

## Fit, Limits, and Trust

ddcore is designed for software teams building business and administrative applications:

- **Open Source:** MIT-licensed, developed openly on [GitHub](https://github.com/jrvidotti/ddcore).
- **Clear Separation:** Business logic runs on the server; form scripts execute in the browser. External side-effects (like outgoing email) are queued and handled by transactional background jobs.
- **Operational Clarity:** The base deployment requires only Go binary + PostgreSQL. External capabilities like PDF rendering (headless browser) or email delivery (SMTP) require configured infrastructure.
- **Evolution:** Check the [Roadmap](https://github.com/jrvidotti/ddcore/blob/main/ROADMAP.md) for planned features such as SSO, MFA, and multi-replica coordination.

Ready to explore? Follow the [Build Your First App](/guide/first-app) tutorial to get started.
