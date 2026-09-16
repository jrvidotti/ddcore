# Design: ddcore Landing Page & Documentation Site on GitHub Pages

Record of the design for the ddcore public website, landing page, and documentation portal.

## Context & Motivation

**ddcore** (*Data Driven Core*) brings the declarative DocType application model of the **Frappe Framework** (the backbone of ERPNext) to a modern, high-performance stack: **Go + TypeScript + PostgreSQL**.

While Frappe relies on Python, WSGI/Gunicorn, MariaDB, and a complex bench environment, ddcore re-engineers this architecture:
- **Compiled Go Engine:** Raw execution speed, microsecond request routing, and minimal memory overhead compared to multiple Python worker processes.
- **End-to-End Type Safety:** TypeScript DocTypes, controllers, and APIs compiled with strict type safety, eliminating dynamic runtime errors.
- **PostgreSQL Native:** Leveraging ACID transactions, advanced indexing, and native JSONB.
- **Single Binary Simplicity:** A single executable embedding `goja` and `esbuild`, requiring only Postgres in production (no Node.js runtime required).
- **Agentic-First Architecture:** Native Model Context Protocol (MCP) server embedded directly inside the framework for AI coding assistants.

The repository already houses 24 comprehensive reference guides in `docs/agent/`. This design establishes a high-performance landing page and documentation site hosted on GitHub Pages (`https://jrvidotti.github.io/ddcore/`), powered by VitePress and automated via GitHub Actions.

---

## Technical Architecture & File Layout

VitePress is deployed directly inside the existing `docs/` directory, maintaining an isolated `docs/package.json` so that dependencies do not pollute the core repository or the desk (`desk/package.json`).

```
ddcore/
├── .github/
│   └── workflows/
│       ├── deploy-docs.yml     # GitHub Actions workflow for Pages
│       └── release.yml
├── docs/
│   ├── .vitepress/
│   │   └── config.mts          # Site configuration, base URL, nav, sidebar, theme
│   ├── agent/                  # (Existing) 24 reference documentation guides
│   │   ├── index.md
│   │   ├── conventions.md
│   │   └── ...
│   ├── embed.go                # (Existing) //go:embed agent/*.md remains untouched
│   ├── index.md                # Landing page with VitePress home layout
│   └── package.json            # VitePress dependency and scripts
├── Makefile                    # Added docs-dev and docs-build shortcuts
└── .gitignore                  # Ignores VitePress cache, dist, and docs/node_modules
```

### Key Compatibility Guarantees:
1. **Go Embedding Isolation:** `docs/embed.go` uses `//go:embed agent/*.md`. Adding `index.md`, `package.json`, and `.vitepress/` under `docs/` does not alter or conflict with the embedded filesystem in the Go binary.
2. **Git Hygiene:** `docs/node_modules`, `docs/.vitepress/dist`, and `docs/.vitepress/cache` are explicitly added to `.gitignore`.

---

## Landing Page Structure (`docs/index.md`)

The landing page leverages VitePress's `layout: home` with custom Markdown sections:

### 1. Hero Section
- **Name:** `ddcore`
- **Text:** `Data Driven Core`
- **Tagline:** *"The declarative power of Frappe (ERPNext) re-engineered in Go + TypeScript + PostgreSQL. Radically faster, low memory footprint, and fully type-safe."*
- **Primary Action:** `text: "Get Started"`, `link: "/agent/"`
- **Secondary Action:** `text: "View on GitHub"`, `link: "https://github.com/jrvidotti/ddcore"`

### 2. Quick Install Block
Prominent copy-pasteable installation snippet:
```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
```

### 3. "Why ddcore? (The Frappe Evolution)" Comparison
A dedicated feature table contrasting traditional Frappe with ddcore:

| Capability | Frappe (Python + MariaDB) | ddcore (Go + TypeScript + Postgres) |
| :--- | :--- | :--- |
| **Runtime & Performance** | Interpreted Python WSGI; high per-request latency | Compiled Go binary; microsecond routing and concurrency |
| **Memory Footprint** | Hundreds of MBs per Gunicorn worker | Minimal RAM footprint (single lightweight binary) |
| **Type Safety** | Dynamic Python runtime | Static typing end-to-end (Go core + strict TS) |
| **Operational Stack** | Bench, Python, virtualenv, Redis, MariaDB, Node | **Single binary:** `./bin/ddcore` + PostgreSQL |
| **AI Agent Era** | UI-centric, human-driven configuration | **Agent-First:** Native MCP server for autonomous coding |

### 4. Six Core Feature Highlights
1. **Single Binary, Zero Node in Production:** Embeds `goja` and `esbuild`. Runs TypeScript controllers synchronously on the server.
2. **Synchronous Server TypeScript:** Expressive business logic running in rolled-back transactions without `await` traps.
3. **Reactive Svelte 5 Desk:** Responsive administrative interface automatically generated from metadata and embedded into the binary.
4. **Agent-First & Native MCP:** Embeds a Model Context Protocol server (`ddcore mcp`) enabling AI agents to scaffold, migrate, test, and query.
5. **Enterprise Batteries Included:** Built-in auth, field permissions (`permlevel`), user access scopes, declarative approval workflows, signed webhooks, encrypted vault, and i18n.
6. **Instant Hot Reload:** `ddcore dev` watches DocTypes, rules, reports, and translation catalogs, reloading in memory without server restarts.

---

## Documentation Navigation & Sidebar (`docs/.vitepress/config.mts`)

The 24 existing documents in `docs/agent/` are categorized into a structured hierarchy:

### Top Navigation Bar:
- **Documentation:** `/agent/`
- **Architecture & Design:** `/agent/conventions`
- **Demo Application:** `https://github.com/jrvidotti/ddcore-demo`
- **GitHub:** `https://github.com/jrvidotti/ddcore`

### Sidebar Sections:
- **Getting Started:**
  - Overview (`/agent/index`)
  - Conventions & App Structure (`/agent/conventions`)
  - CLI & Development Loop (`/agent/cli`)
  - Extending DocTypes (`/agent/extending`)
- **Core Models & APIs:**
  - Fieldtypes Reference (`/agent/fieldtypes`)
  - Server Controller API (`/agent/controller-api`)
  - Desk & Form API (`/agent/form-api`)
  - Reports & Workspaces (`/agent/report-api`)
  - Schema Migrations (`/agent/migrations`)
- **Security & Governance:**
  - Authentication & Passwords (`/agent/auth`)
  - Field Permissions (`/agent/field-permissions`)
  - User Permission Scopes (`/agent/scopes`)
  - Credential Vault (`/agent/vault`)
  - Administrative Audit Trail (`/agent/audit`)
- **Business Logic & Automation:**
  - Declarative Approval Workflows (`/agent/workflows`)
  - Assignments & ToDos (`/agent/assignments`)
  - Event Notifications (`/agent/notifications`)
  - Email Templates & Delivery (`/agent/mail`)
  - Outgoing Webhooks (`/agent/webhooks`)
  - Print Templates & PDF (`/agent/print`)
  - Data Export (`/agent/export`)
- **Operations & Production:**
  - Ops, Health & Observability (`/agent/ops`)
  - Internationalization (`/agent/i18n`)
  - Upstream Feature Requests (`/agent/feature-requests`)

### Search & Theme:
- Built-in local search provider (`provider: 'local'`) for instant client-side indexing.
- Native Light/Dark mode toggling.

---

## Deployment & GitHub Actions Workflow

### 1. Base URL
Configured in `docs/.vitepress/config.mts` to support repository subpaths on GitHub Pages:
```ts
base: process.env.VITEPRESS_BASE || '/ddcore/',
```

### 2. Workflow File (`.github/workflows/deploy-docs.yml`)
- Triggers on push to `main` when changes occur under `docs/**` or `.github/workflows/deploy-docs.yml`.
- Also supports manual execution via `workflow_dispatch`.
- Steps:
  1. `actions/checkout@v4` with `fetch-depth: 0`.
  2. `actions/setup-node@v4` with Node 20 and caching.
  3. `npm --prefix docs install`.
  4. `npm --prefix docs run docs:build`.
  5. `actions/configure-pages@v4`.
  6. `actions/upload-pages-artifact@v3` targeting `docs/.vitepress/dist`.
  7. `actions/deploy-pages@v4`.

### 3. Developer Ergonomics (`Makefile`)
Added Make targets:
```makefile
docs-dev: ## start documentation preview locally (VitePress)
	cd docs && npm install && npm run docs:dev

docs-build: ## build documentation site (VitePress)
	cd docs && npm install && npm run docs:build
```

### 4. GitHub Setup
In the GitHub repository settings:
- Go to **Settings** > **Pages**.
- Under **Build and deployment** > **Source**, select **GitHub Actions**.
- On subsequent pushes to `main`, GitHub Actions builds and publishes the site to `https://jrvidotti.github.io/ddcore/`.
