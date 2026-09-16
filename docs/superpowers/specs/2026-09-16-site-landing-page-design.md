# Design: ddcore Landing Page & Documentation Site on GitHub Pages

Record of the design for the ddcore public website, landing page, and documentation portal.
Revised September 16, 2026 after reviewing framework behavior and the
[outreach plan](../../outreach-plan.md). This specification defines implementation
requirements; it is not evidence that the site or a public demo is already deployed.

## Context & Motivation

**ddcore** (*Data Driven Core*) is an open-source framework for building business
applications from TypeScript models and rules, with a Go runtime, PostgreSQL, a
generated Desk, and built-in MCP development tools. Its declarative DocType model
is inspired by Frappe; this does not imply Frappe or ERPNext compatibility.

The confirmed primary audience is developers and small software studios building
custom business applications. Lead with reduced repetitive work between defining
a business model and delivering a working application:

- **Metadata-driven applications:** DocTypes drive database tables, standard forms,
  lists, REST endpoints, and generated TypeScript interfaces.
- **Go host and embedded Desk:** The binary embeds the HTTP server, Desk, esbuild,
  and goja. Application business logic runs in goja, not as native Go code.
- **Typed development tools:** Typed SDKs and generated DocType interfaces support
  TypeScript checks during development. esbuild transpiles without typechecking;
  typechecks are separate and do not eliminate runtime errors.
- **Base deployment:** Run the binary with the application's TypeScript files and
  PostgreSQL, without a Node.js production server. Email requires a configured
  transport; server-side PDF generation requires an external renderer.
- **Agent-assisted development:** MCP exposes documentation, metadata, scaffolding,
  migrations, records, and tests to coding agents.

There are no measured performance results supporting speed or memory comparisons
in this specification. Do not publish numerical claims, “radically faster,”
“minimal memory,” or “fully type-safe.” A future benchmark must identify the
release, hardware, workload, dataset, concurrency, error rate, and methodology.

The existing Markdown reference in `docs/agent/` remains canonical. Publish it
alongside human-oriented guides using VitePress and GitHub Actions at the intended
GitHub Pages custom domain, `https://ddcore.dev`.

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
│   ├── agent/                  # Existing canonical reference, also embedded in Go
│   │   ├── index.md
│   │   ├── conventions.md
│   │   └── ...
│   ├── guide/                  # New human-oriented learning paths
│   │   ├── first-app.md
│   │   ├── architecture.md
│   │   ├── demo.md
│   │   └── deployment.md
│   ├── embed.go                # (Existing) //go:embed agent/*.md remains untouched
│   ├── index.md                # Landing page with VitePress home layout
│   ├── package.json            # VitePress dependency and scripts
│   └── package-lock.json       # Committed, reproducible dependency resolution
├── Makefile                    # docs-dev, docs-build, and docs-preview shortcuts
└── .gitignore                  # Ignores VitePress cache, dist, and docs/node_modules
```

### Key Compatibility Guarantees:

1. **Go Embedding Isolation:** `docs/embed.go` uses `//go:embed agent/*.md`. Adding `index.md`, `package.json`, and `.vitepress/` under `docs/` does not alter or conflict with the embedded filesystem in the Go binary.
2. **Git Hygiene:** `docs/node_modules`, `docs/.vitepress/dist`, and `docs/.vitepress/cache` are explicitly added to `.gitignore`.
3. **Canonical Reference:** Render `docs/agent/` directly. Avoid duplicating it or
   introducing Vue-only components into Markdown consumed by the CLI and MCP.

### Public Content Boundary

The initial public Markdown set is `index.md`, `agent/**/*.md`, and the four
`guide/` pages listed above. A sidebar is navigation, not a publication filter.
Exclude internal planning material explicitly, including this specification:

```ts
srcExclude: [
  'superpowers/**',
  'outreach-plan.md',
  'frappe-port-inventory.md',
  'frappe-implemented-features.md',
],
ignoreDeadLinks: false,
```

During implementation, audit all Markdown under `docs/` and extend exclusions
for any other non-public files. The build validation must reject page inputs
outside the approved public set, so newly added internal documents cannot silently
become public pages. Confirm excluded material is absent from generated pages and
the local search index. These exclusions keep draft content out of the website;
they do not make files in the public repository confidential.

Links to source code or repository documents outside the public set must use
GitHub URLs tied to the relevant revision. Do not disable dead-link checks to
accommodate repository-relative links that cannot resolve on the site.

---

## Landing Page Structure (`docs/index.md`)

The landing page leverages VitePress's `layout: home` with custom Markdown sections:

### 1. Hero Section

- **Name:** `ddcore`
- **Text:** `Build business applications from TypeScript models.`
- **Tagline:** `Define your data and business rules. ddcore provides database tables, forms, lists, APIs, and access controls, with a Go runtime and built-in MCP tools for agent-assisted development.`
- **Primary Action:** `text: "Build your first app"`, `link: "/guide/first-app"`
- **Secondary Action:** `text: "Explore the example app"`, `link: "https://github.com/jrvidotti/ddcore-demo"`
- **Supporting Line:** `MIT-licensed · Go + TypeScript + PostgreSQL · Self-hosted`

Keep GitHub available in the navigation. Use “Try the demo” only when a public
deployment is verified and its URL is configured; a repository link is an example
app, not an interactive demo. Copy is authored in English. Portuguese localization
can follow without delaying a complete English learning path.

### 2. Quick Install Block
Prominent copy-pasteable CLI installation snippet:
```bash
curl -fsSL https://raw.githubusercontent.com/jrvidotti/ddcore/main/install.sh | sh
```

Label this as installation of the CLI, not a complete application setup. Link to
the first-app guide for supported platforms, PostgreSQL, PATH setup, a pinned
release, and the remaining steps. Do not promise a setup duration before testing
the flow with new developers.

### 3. From a Model to a Working Application

Show a small real DocType excerpt beside an actual screenshot of the form or list
it produces. Identify the example app and revision. Follow it with a short
project/task flow: update a task, observe derived project progress, trigger a
server-side validation, and open the report. Link to `/guide/demo` and the source.

Use `ddcore-demo` as the executable tutorial; preserve `apps/testapp` as the small
acceptance fixture. Do not imply that every framework feature is already exercised
by the demo. Approval, mail, or webhook segments require their own verified app
implementation before they appear in the walkthrough.

Frappe context belongs in the architecture guide. Describe the shared DocType idea
and ddcore's concrete runtime and development choices with sources. Avoid claims
about Frappe latency, worker memory, or lack of agent tooling without evidence,
and do not promise drop-in compatibility.

### 4. Six Core Feature Highlights

1. **One Model, Several Application Layers:** DocTypes drive tables, forms, lists,
   REST endpoints, and generated TypeScript interfaces.
2. **Server-side Business Rules:** Synchronous TypeScript with transactional writes,
   commit on success, and rollback on failure. Tests use rolled-back transactions.
   External effects such as sent email do not roll back with the database.
3. **Generated Desk:** A Svelte 5 administrative interface embedded in the binary,
   with standard forms, lists, reports, and workspaces.
4. **Built-in MCP Development Tools:** Agents can inspect metadata, scaffold,
   migrate, query, and test. Source editing uses the agent's normal file tools;
   MCP does not supply an autonomous language model or guarantee correct code.
5. **Business Application Features:** Authentication, field permissions, user
   scopes, approval workflows, signed webhooks, encrypted vault, and localization.
   Link to the contracts and their limitations.
6. **Simple Runtime and Development Loop:** Binary, app files, and PostgreSQL form
   the base deployment. `ddcore dev` reloads app changes; metadata changes require
   migration and type generation. No Node.js production server is needed.

### 5. Fit, Limits, and Trust

Link to the license, source, roadmap, and deployment guide. Explain that business
application capabilities are not a blanket enterprise-readiness guarantee. The
deployment guide must distinguish implemented tooling from operator responsibilities
and roadmap items such as recovery automation, SSO/MFA, and multiple replicas.

Explain the server/browser TypeScript split, trusted custom server code's permission
responsibilities, and optional mail/PDF dependencies. MCP is privileged development
tooling: stdio runs through `ddcore mcp`; HTTP MCP is mounted in `dev` and requires
an administrative API key. It is not an end-user assistant or a public demo feature.

---

## Documentation Navigation & Sidebar (`docs/.vitepress/config.mts`)

Keep the existing reference hierarchy and add human learning paths above it.

### Top Navigation Bar:

- **Get Started:** `/guide/first-app`
- **Documentation:** `/agent/`
- **Architecture & Design:** `/guide/architecture`
- **Demo Application:** `https://github.com/jrvidotti/ddcore-demo`
- **GitHub:** `https://github.com/jrvidotti/ddcore`

### Sidebar Sections:

- **Getting Started:**
  - Build Your First App (`/guide/first-app`)
  - Architecture & Mental Model (`/guide/architecture`)
  - Example App Walkthrough (`/guide/demo`)
  - Deployment & Production Checklist (`/guide/deployment`)
- **Reference Overview:**
  - Overview (`/agent/`)
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

- Built-in local search provider (`provider: 'local'`); index only published content.
- Native Light/Dark mode toggling.
- Keyboard-accessible navigation/search and readable layouts on mobile.

### First-App Guide Contract

The primary CTA must lead to a tested sequence using the published binary, without
cloning or building the framework: install a pinned release, configure PostgreSQL,
initialize an app, declare a DocType, migrate, set up login, open its form, add a
business validation, and run a test. Include prerequisites, exact commands,
expected output, and recovery from common setup errors. Separate app development
from framework contribution. Link to the reference instead of copying its API
contracts. Do not ship empty guide pages or navigation to planned pages.

### Documentation Version Policy

The initial site follows `main` and must display “Development documentation (main)”
with the source revision. The first-app and demo guides must state the exact
released binary and example revision they were tested against. Mark features not
yet available in that release explicitly; landing claims must be available in the
release recommended to visitors. Do not label development reference as stable.
Versioned documentation can be added later; it is not required for this launch.

---

## Deployment & GitHub Actions Workflow

### 1. Base URL
Configured in `docs/.vitepress/config.mts` to support repository subpaths on GitHub Pages:

```ts
base: process.env.VITEPRESS_BASE || '/ddcore/',
```

### 2. Workflow File (`.github/workflows/deploy-docs.yml`)

- Build on pull requests targeting `main`, pushes to `main`, and `workflow_dispatch`.
  Path filters include `docs/**`, the workflow, and `Makefile`.
- Use separate build and deploy jobs. Build permissions are `contents: read`;
  only deployment gets `pages: write` and `id-token: write` in addition to read.
- Deploy only after a successful build, for a push to `main` or manual execution
  on `main`. Pull requests and manual runs from other refs must not deploy.
- The deploy job uses the `github-pages` environment, the deployment output URL,
  and a shared deployment concurrency group with `cancel-in-progress: false`.
- Use supported, explicitly pinned action versions and Node 24 LTS. Commit
  `docs/package-lock.json`; configure npm caching with
  `cache-dependency-path: docs/package-lock.json`.
- Build steps: checkout; set up Node; `npm --prefix docs ci`;
  `npm --prefix docs run docs:build`; validate the public page set and output.
  Use `fetch-depth: 0` only when git-based last-updated metadata is enabled.
  For eligible publishing runs, preserve `docs/.vitepress/dist` with
  `actions/upload-artifact` after validation.
- The deploy job depends on the build job, downloads that run's artifact with
  `actions/download-artifact` into `docs/.vitepress/dist`, and uses
  `actions/configure-pages`, `actions/upload-pages-artifact`, and
  `actions/deploy-pages`. Do not rebuild different content for deployment.
- PR validation must work without deployment secrets or Pages write permissions.

Pin a stable VitePress version compatible with the chosen Node version; do not
select an alpha release implicitly. With `docs/package.json` as the package root,
scripts are `vitepress dev .`, `vitepress build .`, and `vitepress preview .` for
`docs:dev`, `docs:build`, and `docs:preview`, respectively.

### 3. Developer Ergonomics (`Makefile`)
Added Make targets:

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

`docs-preview` assumes a successful `docs-build`. Keep docs dependencies separate
from Desk dependencies. The docs build is an explicit PR check; it does not replace
the repository's required `make test` validation.

### 4. GitHub Setup

In the GitHub repository settings:

- Go to **Settings** > **Pages**.
- Under **Build and deployment** > **Source**, select **GitHub Actions**.
- Verify the `github-pages` environment permits deployment from `main`.
- Successful eligible workflows publish to `https://ddcore.dev` (custom domain; `docs/public/CNAME`).
  Repository configuration and the live deployment must be verified separately
  from generating the workflow file.

## Acceptance Criteria

- All landing claims map to implemented behavior in the recommended release.
  No unsupported performance, memory, compatibility, or runtime-safety guarantees.
- `make docs-build` succeeds from a clean dependency installation with the committed
  lockfile. Dead-link checks remain enabled.
- Only the approved Markdown pages are rendered/indexed. Internal specs, plans,
  inventories, and the outreach brief are absent from pages and search results.
- Preview the production build under `/ddcore/`: check landing CTAs, direct page
  loads, refreshes, sidebar links, images, code blocks, search, and the 404 page.
- Check mobile and desktop layouts, keyboard navigation, focus visibility, and
  light/dark readability. The DocType excerpt and screenshot describe the same app.
- Complete the first-app guide using its stated released binary, without a local
  framework checkout. Verify the demo guide against its linked app revision.
- Display development/release context consistently; no empty guides, invented
  demo URLs, or claims based only on unreleased source.
- PR builds do not deploy. A successful authorized `main` build can deploy the
  same validated artifact, with Pages permissions scoped to deployment.
- Preserve the embedded reference contract and run `make test` for repository
  changes. Site build, browser checks, and actual deployment are separate checks;
  passing framework tests alone does not establish that the website works.

## Implementation References

- [Outreach plan and claim evidence](../../outreach-plan.md)
- [Framework mental model](../../../DEVELOPMENT.md)
- [Framework roadmap and operational limits](../../../ROADMAP.md)
- [VitePress source exclusions](https://vitepress.dev/reference/site-config#srcexclude)
- [VitePress deployment](https://vitepress.dev/guide/deploy#github-pages)
- [esbuild TypeScript behavior](https://esbuild.github.io/content-types/#typescript)
- [Node.js supported releases](https://nodejs.org/en/about/previous-releases)
