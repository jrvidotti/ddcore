# ddcore Landing Page & Documentation Site Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Create and configure a VitePress-based landing page and documentation website for ddcore inside `docs/`, integrate human-oriented guides and the canonical agent reference, and automate deployment to GitHub Pages via GitHub Actions.

**Architecture:** VitePress runs in `docs/` with its own isolated `package.json`. The site includes a modern landing page (`docs/index.md`), four new human learning guides (`docs/guide/`), and mounts the canonical `docs/agent/` reference. Excludes internal specifications and planning documents. Built with GitHub Actions into GitHub Pages artifacts.

**Tech Stack:** VitePress 1.x, Node 20+, GitHub Actions, Make

**Spec:** `docs/superpowers/specs/2026-09-16-site-landing-page-design.md`

## Global Constraints

- No unmeasured claims (“radically faster”, “minimal memory”, “fully type-safe”).
- English is the canonical language for all documentation and site copy.
- Go embedding isolation: `docs/embed.go` (`//go:embed agent/*.md`) must not be broken or altered.
- `ignoreDeadLinks: false` — all links in the public documentation set must resolve cleanly.
- VitePress `base`: `process.env.VITEPRESS_BASE || '/ddcore/'`.
- Exclude internal planning documents: `superpowers/**`, `outreach-plan.md`, `frappe-port-inventory.md`, `frappe-implemented-features.md`.
- Separate build and deploy jobs in `.github/workflows/deploy-docs.yml`.

---

### Task 1: Scaffolding VitePress, package.json, .gitignore, and Makefile

**Files:**
- Create: `docs/package.json`
- Modify: `.gitignore`
- Modify: `Makefile:70-75`

**Interfaces:**
- Consumes: Node.js / npm
- Produces: `docs/package-lock.json`, `npm --prefix docs run docs:dev`, `make docs-dev`, `make docs-build`

- [ ] **Step 1: Create `docs/package.json`**

Create `docs/package.json` with pinned stable VitePress dependencies:
```json
{
  "name": "ddcore-docs",
  "version": "0.1.0",
  "private": true,
  "type": "module",
  "scripts": {
    "docs:dev": "vitepress dev .",
    "docs:build": "vitepress build .",
    "docs:preview": "vitepress preview ."
  },
  "devDependencies": {
    "vitepress": "^1.6.3"
  }
}
```

- [ ] **Step 2: Install dependencies and generate `docs/package-lock.json`**

Run: `npm --prefix docs install`
Verify `docs/package-lock.json` is generated.

- [ ] **Step 3: Update `.gitignore`**

Append VitePress build and cache directories to `.gitignore`:
```gitignore
docs/.vitepress/dist
docs/.vitepress/cache
docs/node_modules
```

- [ ] **Step 4: Add documentation targets to `Makefile`**

Add targets to `Makefile`:
```makefile
.PHONY: docs-dev docs-build docs-preview

docs-dev: ## start documentation preview locally (VitePress)
	npm --prefix docs ci
	npm --prefix docs run docs:dev

docs-build: ## build documentation site (VitePress)
	npm --prefix docs ci
	npm --prefix docs run docs:build

docs-preview: ## preview the built documentation site
	npm --prefix docs run docs:preview
```

- [ ] **Step 5: Commit**

```bash
git add docs/package.json docs/package-lock.json .gitignore Makefile
git commit -m "chore(docs): scaffold vitepress and makefile targets"
```

---

### Task 2: Configure VitePress (`docs/.vitepress/config.mts`)

**Files:**
- Create: `docs/.vitepress/config.mts`

**Interfaces:**
- Consumes: `docs/index.md`, `docs/agent/*.md`, `docs/guide/*.md`
- Produces: VitePress configuration with navbar, sidebar, search, base path, and content exclusions

- [ ] **Step 1: Create `docs/.vitepress/config.mts`**

Configure title, description, base, `srcExclude`, `ignoreDeadLinks: false`, local search, nav, and sidebar according to the specification:
```ts
import { defineConfig } from 'vitepress'

export default defineConfig({
  title: 'ddcore',
  description: 'Data Driven Core — Build business applications from TypeScript models',
  base: process.env.VITEPRESS_BASE || '/ddcore/',
  srcExclude: [
    'superpowers/**',
    'outreach-plan.md',
    'frappe-port-inventory.md',
    'frappe-implemented-features.md',
  ],
  ignoreDeadLinks: false,
  themeConfig: {
    siteTitle: 'ddcore',
    nav: [
      { text: 'Get Started', link: '/guide/first-app' },
      { text: 'Documentation', link: '/agent/' },
      { text: 'Architecture', link: '/guide/architecture' },
      { text: 'Demo App', link: 'https://github.com/jrvidotti/ddcore-demo' },
      { text: 'GitHub', link: 'https://github.com/jrvidotti/ddcore' }
    ],
    sidebar: [
      {
        text: 'Getting Started',
        items: [
          { text: 'Build Your First App', link: '/guide/first-app' },
          { text: 'Architecture & Mental Model', link: '/guide/architecture' },
          { text: 'Example App Walkthrough', link: '/guide/demo' },
          { text: 'Deployment & Production', link: '/guide/deployment' }
        ]
      },
      {
        text: 'Reference Overview',
        items: [
          { text: 'Overview', link: '/agent/' },
          { text: 'Conventions & Structure', link: '/agent/conventions' },
          { text: 'CLI & Development Loop', link: '/agent/cli' },
          { text: 'Extending DocTypes', link: '/agent/extending' }
        ]
      },
      {
        text: 'Core Models & APIs',
        items: [
          { text: 'Fieldtypes Reference', link: '/agent/fieldtypes' },
          { text: 'Server Controller API', link: '/agent/controller-api' },
          { text: 'Desk & Form API', link: '/agent/form-api' },
          { text: 'Reports & Workspaces', link: '/agent/report-api' },
          { text: 'Schema Migrations', link: '/agent/migrations' }
        ]
      },
      {
        text: 'Security & Governance',
        items: [
          { text: 'Authentication & Passwords', link: '/agent/auth' },
          { text: 'Field Permissions', link: '/agent/field-permissions' },
          { text: 'User Access Scopes', link: '/agent/scopes' },
          { text: 'Credential Vault', link: '/agent/vault' },
          { text: 'Administrative Audit Trail', link: '/agent/audit' }
        ]
      },
      {
        text: 'Business Logic & Automation',
        items: [
          { text: 'Approval Workflows', link: '/agent/workflows' },
          { text: 'Assignments & ToDos', link: '/agent/assignments' },
          { text: 'Event Notifications', link: '/agent/notifications' },
          { text: 'Email Templates & Delivery', link: '/agent/mail' },
          { text: 'Outgoing Webhooks', link: '/agent/webhooks' },
          { text: 'Print Templates & PDF', link: '/agent/print' },
          { text: 'Data Export', link: '/agent/export' }
        ]
      },
      {
        text: 'Operations & Production',
        items: [
          { text: 'Ops, Health & Observability', link: '/agent/ops' },
          { text: 'Internationalization (i18n)', link: '/agent/i18n' },
          { text: 'Upstream Feature Requests', link: '/agent/feature-requests' }
        ]
      }
    ],
    search: {
      provider: 'local'
    },
    socialLinks: [
      { icon: 'github', link: 'https://github.com/jrvidotti/ddcore' }
    ],
    footer: {
      message: 'MIT Licensed · Development documentation (main)',
      copyright: 'Copyright © 2026 ddcore contributors'
    }
  }
})
```

- [ ] **Step 2: Commit**

```bash
git add docs/.vitepress/config.mts
git commit -m "feat(docs): add vitepress configuration with navigation and exclusions"
```

---

### Task 3: Landing Page Implementation (`docs/index.md`)

**Files:**
- Create: `docs/index.md`

**Interfaces:**
- Consumes: Spec requirements for Hero, Quick Install, Model-to-App, 6 Core Features, Limits & Trust
- Produces: VitePress home page (`docs/index.md`)

- [ ] **Step 1: Write `docs/index.md`**

Write `docs/index.md` strictly aligned with the spec:
- Hero: title `ddcore`, text `Build business applications from TypeScript models.`, tagline, action buttons.
- Install snippet: `curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh` with explicit note that this installs the CLI, linking to `/guide/first-app`.
- "From a Model to a Working Application" walkthrough using real DocType code from `ddcore-demo` (Task / Project).
- 6 Core Feature Highlights matching the spec (One Model, Server-side Business Rules, Generated Desk, Built-in MCP Tools, Business Features, Simple Runtime).
- Fit, Limits, and Trust section: MIT-licensed, developer-oriented, distinguishing implemented capabilities from operator responsibilities.

- [ ] **Step 2: Commit**

```bash
git add docs/index.md
git commit -m "feat(docs): implement landing page"
```

---

### Task 4: Author Human Learning Guides (`docs/guide/*.md`)

**Files:**
- Create: `docs/guide/first-app.md`
- Create: `docs/guide/architecture.md`
- Create: `docs/guide/demo.md`
- Create: `docs/guide/deployment.md`

**Interfaces:**
- Consumes: `DEVELOPMENT.md`, `README.md`, `ROADMAP.md`, `docs/agent/*`
- Produces: 4 comprehensive learning guides referenced by top nav and sidebar

- [ ] **Step 1: Author `docs/guide/first-app.md`**
Step-by-step tutorial: prerequisites (Go or CLI binary, Postgres), `ddcore init`, `ddcore new-app library`, declaring `book.doctype.ts`, running `ddcore migrate`, creating admin user, testing in Desk, adding validation in `book.controller.ts`, running `ddcore test`.

- [ ] **Step 2: Author `docs/guide/architecture.md`**
Explains the mental model: DocType declarative concept inspired by Frappe, Go host embedding goja and esbuild, Svelte 5 Desk, synchronous TypeScript execution on server, transactional boundaries, agent-first MCP server.

- [ ] **Step 3: Author `docs/guide/demo.md`**
Walkthrough of `ddcore-demo` (Projects, Tasks, Timesheets): DocType relationships, calculated fields, server hooks, workspace charts. Links to GitHub repo.

- [ ] **Step 4: Author `docs/guide/deployment.md`**
Production deployment guide: system requirements, environment variables (`DDCORE_DSN`, `DDCORE_PORT`, `DDCORE_SECRET_KEY`), systemd service unit, Docker Compose deployment, reverse proxy (Nginx/Caddy) with TLS, backup/restore strategy, operational checklists.

- [ ] **Step 5: Commit**

```bash
git add docs/guide/
git commit -m "feat(docs): add human learning guides"
```

---

### Task 5: Link Audit and Reference Navigation Harmonization

**Files:**
- Check: all files in `docs/agent/*.md` and `docs/guide/*.md`
- Modify: any broken relative links that fail `ignoreDeadLinks: false`

**Interfaces:**
- Consumes: VitePress link checker (`vitepress build .`)
- Produces: Zero dead-link warnings or build errors

- [ ] **Step 1: Test build with VitePress**
Run: `npm --prefix docs run docs:build`
Inspect any broken links reported by VitePress.

- [ ] **Step 2: Fix relative links in `docs/agent/*.md` or `docs/guide/*.md`**
Ensure links to other docs resolve to `.md` or route paths. Any references to files outside the public docs (like `DEVELOPMENT.md` or `ROADMAP.md`) must use GitHub links (`https://github.com/jrvidotti/ddcore/blob/main/DEVELOPMENT.md`).

- [ ] **Step 3: Verify build passes cleanly**
Run: `npm --prefix docs run docs:build`
Expected: Succeeded with 0 errors.

- [ ] **Step 4: Commit**

```bash
git add docs/
git commit -m "fix(docs): harmonize markdown links for clean vitepress build"
```

---

### Task 6: GitHub Actions Documentation Deployment Workflow

**Files:**
- Create: `.github/workflows/deploy-docs.yml`

**Interfaces:**
- Consumes: `docs/package-lock.json`, `docs/`
- Produces: GitHub Pages deployment artifact and deployment on push to `main`

- [ ] **Step 1: Create `.github/workflows/deploy-docs.yml`**

Implement separate `build` and `deploy` jobs:
- Trigger on `push` to `main`, `pull_request` to `main` (paths: `docs/**`, `.github/workflows/deploy-docs.yml`, `Makefile`), and `workflow_dispatch`.
- `build` job:
  - Permissions: `contents: read`
  - Runs on `ubuntu-latest`
  - Checkout (`fetch-depth: 0`)
  - Setup Node 20 with npm caching (`cache-dependency-path: docs/package-lock.json`)
  - `npm --prefix docs ci`
  - `npm --prefix docs run docs:build`
  - If ref is `refs/heads/main` and event != `pull_request`, upload artifact `docs/.vitepress/dist` via `actions/upload-artifact@v4`
- `deploy` job:
  - Needs `build`
  - Runs only on `refs/heads/main` and event != `pull_request`
  - Permissions: `contents: read`, `pages: write`, `id-token: write`
  - Environment: `github-pages`
  - Download artifact via `actions/download-artifact@v4` into `docs/.vitepress/dist`
  - `actions/configure-pages@v4`
  - `actions/upload-pages-artifact@v3` with path `docs/.vitepress/dist`
  - `actions/deploy-pages@v4`

- [ ] **Step 2: Commit**

```bash
git add .github/workflows/deploy-docs.yml
git commit -m "ci(docs): add github pages build and deploy workflow"
```

---

### Task 7: Full Verification and Framework Integrity Checks

**Files:**
- None (verification only)

**Interfaces:**
- Consumes: `make docs-build`, `make check`, `make test`, `git status`
- Produces: Verification evidence that framework and documentation are fully functional

- [ ] **Step 1: Run `make docs-build`**
Run: `make docs-build`
Expected: Output created in `docs/.vitepress/dist`, 0 dead links.

- [ ] **Step 2: Verify Go build and tests are untouched**
Run: `make vet` and `go build ./...`
Verify that `docs/embed.go` embeds `agent/*.md` without issues.

- [ ] **Step 3: Run `make check`**
Run: `make check`
Expected: Desk typecheck and i18n checks pass.

- [ ] **Step 4: Verify search index and public boundary**
Inspect `docs/.vitepress/dist` to confirm no files from `superpowers/`, `outreach-plan.md`, or inventories leaked into output HTML or search index.
