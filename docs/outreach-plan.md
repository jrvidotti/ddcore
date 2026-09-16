# ddcore: product evidence and outreach plan

Prepared: September 16, 2026. Framework checkout reviewed: `2ff46c1`.

This is a proposed positioning and adoption plan, plus a content brief for the
landing page being developed separately. It does not specify the page's visual
design or authorize publishing. Product facts are grounded in this checkout;
copy, sequencing, and campaign targets below are recommendations.

The user-confirmed primary audience is developers and small software studios
building custom business applications.
All public capability claims must be checked against the release visitors will
actually install: a feature present on this branch may not be in that release.

## 1. Recommended positioning

**ddcore is an open-source framework for building business applications from
TypeScript models and rules, with a Go runtime, PostgreSQL, a generated Desk,
and built-in MCP development tools.**

Lead with the developer's outcome: fewer repeated implementation tasks between a
business model and a working application. Explain Go and MCP as the mechanisms
that support deployment and development. Performance numbers should become a
proof point only after measurement.

### Three possible approaches

| Approach | Strength | Tradeoff | Recommendation |
| --- | --- | --- | --- |
| Business applications from TypeScript | Connects forms, permissions, rules, reports, and deployment to a recognizable problem | Needs a complete business-flow demo | Primary positioning |
| Business applications built with AI agents | The native MCP workflow is concrete and demonstrable | Can attract expectations of autonomous no-code generation | Secondary message and a dedicated tutorial |
| A Go alternative inspired by Frappe | Gives experienced Frappe developers a familiar mental model | Invites compatibility, maturity, and speed assumptions | Technical comparison content |

Frappe already documents metadata-driven Desk behavior, role permissions, and
generated REST APIs. Those ideas are category context, not claims of uniqueness.
The ddcore story is their particular combination with file-based TypeScript apps,
an embedded Go host, and development tooling exposed through MCP. Do not claim
exclusivity without a broader competitive review.
Source: [Frappe introduction](https://docs.frappe.io/framework/user/en/introduction).

### Audience and fit

- **First audience:** TypeScript developers and small studios repeatedly building
  administrative systems, operational tools, and domain-specific back offices.
- **Next audience:** internal engineering teams with a concrete workflow and the
  ability to assess their own production requirements.
- **Amplifiers:** developers interested in metadata-driven software, Frappe, Go
  deployment, and agent-assisted development.

Good initial use cases include project operations, service requests, approval
flows, and internal records with reports. These are application opportunities,
not prebuilt products included with ddcore.

The current model is less suitable when the central requirement is a completely
bespoke public-facing interface, Node-specific server dependencies, heavy
JavaScript computation, or already-established multi-tenant/multi-replica hosting.
Corporate application features should not be presented as certification of
enterprise operational readiness.

## 2. Evidence and public claims

The following is a copy reference, not a substitute for the linked API contracts.

| Capability | Evidence | Public wording supported by this checkout | Qualification |
| --- | --- | --- | --- |
| Models become applications | [Development model](../DEVELOPMENT.md), [agent index](agent/index.md) | “Define DocTypes in TypeScript. Get database tables, forms, lists, and REST endpoints.” | App-specific rules still require code; schema changes require migration. |
| Embedded runtime and Desk | [Development model](../DEVELOPMENT.md), [release workflow](../.github/workflows/release.yml) | “Run your app with the ddcore binary and PostgreSQL.” | Ship the app's TypeScript files too. This does not mean the whole app and database are one executable. |
| No Node production runtime | [Development model](../DEVELOPMENT.md) | “Write TypeScript business logic without running a Node.js server in production.” | Node is used in development/typechecking and to build the framework's Desk. |
| Business rules and transactions | [Controller API](agent/controller-api.md) | “Keep business validation on the server and database writes inside transactions.” | External HTTP or email effects are not undone by a database rollback. |
| Access controls | [Scopes](agent/scopes.md), [field permissions](agent/field-permissions.md), [roadmap](../ROADMAP.md) | “Declare role permissions, user access scopes, and field access levels.” | Custom trusted server code has explicit responsibilities and bypass paths; do not say every custom output is automatically filtered. |
| Approvals and daily work | [Workflows](agent/workflows.md), [assignments](agent/assignments.md), [notifications](agent/notifications.md) | “Build approval flows, assignments, and notifications into your application.” | Declarative workflows are state machines; no visual workflow builder is promised. |
| Reports and workspaces | [Report API](agent/report-api.md) | “Declare reports, number cards, and workspace charts in TypeScript.” | This is not a promise of a no-code BI designer. |
| Integrations | [Mail](agent/mail.md), [webhooks](agent/webhooks.md) | “Connect business events to email and signed outgoing webhooks.” | Delivery can be retried; external effects require idempotency. Mail needs a configured transport. |
| Printing | [Print API](agent/print.md) | “Generate printable documents and configure server-side PDF output.” | PDF generation needs Gotenberg, a compatible command, or local Chromium. |
| Development loop | [CLI](agent/cli.md), [development guide](../DEVELOPMENT.md) | “Scaffold models, reload app code, generate types, and test rules with the CLI.” | Server TypeScript is synchronous; generated typings are not edited manually. |
| MCP | [Server implementation](../internal/mcp/mcp.go), [CLI mounting](../cmd/ddcore/main.go) | “Give your coding agent access to documentation, metadata, migrations, records, and tests.” | Development/admin tooling; it runs privileged operations and is not an end-user assistant. |
| Operations | [Operations](agent/ops.md) | “Inspect health, background jobs, request IDs, and errors with built-in tools.” | No Prometheus/OTel endpoint or general high-availability guarantee. |
| Ownership | [License](../LICENSE), [standalone app model](../DEVELOPMENT.md) | “MIT-licensed. Keep your application code in your own repository.” | Licensing does not promise support, compatibility with other frameworks, or zero migration effort. |

### Wording to keep out of the landing page

- “10x faster,” “uses almost no memory,” or any throughput/latency claim without
  reproducible results for a named release and workload.
- “TypeScript compiled to native Go.” esbuild transforms the app code; goja runs it.
- “Build any app without code” or “AI builds production-ready systems automatically.”
- “Drop-in Frappe replacement” or “fully compatible with ERPNext.”
- “Enterprise-ready security,” “automatic compliance,” or “complete tenant isolation.”
- “Only PostgreSQL is ever needed.” It is the base runtime dependency; optional
  capabilities such as mail and server-side PDFs need additional configuration/services.
- “Any npm package works.” The server runtime is not Node and has no useful async
  event loop for app code.
- “Multi-tenant SaaS ready,” “horizontal scaling built in,” or “exactly-once jobs.”

## 3. Technical assessment

### Go binary and performance

The proven benefit is packaging and operational simplicity. The binary embeds the
Desk, HTTP server, TypeScript tooling, and JavaScript runtime. Release automation
builds binaries for Linux and macOS on amd64 and arm64 with `CGO_ENABLED=0`.
Applications load their files into this host without compiling their own Go server.

The [runtime pool](../internal/js/runtime.go) reuses goja VMs and bounds concurrent
checkouts with a semaphore. The [engine](../internal/engine/engine.go) currently
sizes the pool as `Workers + 4`. Database access uses pgx's connection pool.
These are implementation facts, not evidence of a particular capacity.

Performance depends on database queries, permission checks, app hooks, the
Go/JavaScript JSON bridge, and available runtime slots. Synchronous external I/O
can hold a runtime while it waits. Creating additional runtimes under initial
load can behave differently from serving a warmed workload. Heavy app computation
still executes in goja.

No benchmark suite or published p95/p99/throughput results were found in this
checkout. No load test was run for this analysis. Functional tests establish
behavior, not capacity.

**Benchmark deliverable before numerical performance copy:**

| Scenario | What to measure |
| --- | --- |
| Process startup to readiness | Cold and warm startup; database readiness recorded separately |
| Authenticated filtered list | p50/p95/p99, throughput, error rate, query count |
| Save with validation and child rows | Transaction latency, rollback behavior, throughput |
| Report over a seeded dataset | Query plan, latency, CPU and memory |
| Mixed foreground traffic and jobs | Queue age, user-facing latency, runtime/database pool saturation |
| Sustained load and recovery | RSS, CPU, error rate, recovery after a burst |

Use an isolated environment with a pinned release, hardware limits, Go/Postgres
versions, dataset sizes, indexes, pool settings, and load scripts. Include several
concurrency levels, warmup, repeated runs, and error rates. Measure the ddcore
process and database separately. Publish raw results and reproduction steps.
A health endpoint alone is not a business-application benchmark. Compare another
framework only with equivalent business behavior and deployment resources.

### MCP and agent-assisted development

The MCP server exposes a practical development loop: read embedded reference
resources, inspect metadata, scaffold a DocType, validate metadata, preview/apply
migrations, generate types, exercise records and methods, inspect errors, and run
tests. Translation tools and job administration extend that loop.

The TypeScript source files remain the source of truth. The coding agent's normal
file-editing tools write controllers and services; MCP supplies structured
inspection and execution. Avoid implying that MCP itself includes arbitrary
source editing or an embedded language model.

The strongest demonstration is observable: introduce a field and rule, apply the
migration, trigger the validation, run a test, and show the resulting screen.
Evaluate saved developer effort through observed tasks, not a generic AI claim.

Operational boundary: stdio is available through `ddcore mcp`; the CLI mounts HTTP
MCP during `dev` behind an Administrator/System Manager API key. Tool execution
uses Administrator authority. Public demo visitors should receive a normal app
experience, not access to this development endpoint.

### Business application depth and maturity

The feature set extends beyond CRUD: transactional lifecycle hooks, child tables,
document submission/cancellation, roles, scopes, field levels, approvals,
assignments, notifications, reports, background work, signed webhooks, printing,
audit records, and localization provide credible material for business demos.

The [roadmap](../ROADMAP.md) also names material residual work: backup/restore and
recovery automation, resumable import and reconciliation, core/app compatibility
contracts, SSO/MFA, multisite, and same-tenant replicas. Access-control caveats
include trusted custom server outputs and specific scope/attachment paths.
Prospective production adopters need a release-specific checklist tied to their
critical flows. A feature inventory is not a completed production audit.

### Ease of development

The main reduction in work comes from sharing one metadata definition across
storage, standard UI, API, and generated types. File conventions, scaffolding,
hot reload, transaction-based tests, and embedded documentation make the workflow
predictable for people and agents.

Teach the constraints early: synchronous server TypeScript versus asynchronous
browser scripts; DocType lifecycle; framework/app responsibility; server-side
validation; permissions on custom outputs; migrations; canonical English strings
and translation catalogs. Explain where the generated Desk fits and where a
different interface is needed.

The current framework README moves from binary installation to building the
framework. A new app author needs a separate, direct path using the published
binary. This onboarding improvement should precede broad distribution.

## 4. Content brief for the landing-page agent

This section supplies content and evidence. Layout and implementation belong to
the agent already building the landing page.

### Proposed hero copy

**Headline:** Build business applications from TypeScript models.

**Supporting copy:** Define your data and business rules. ddcore provides database
tables, forms, lists, APIs, and access controls, with a Go runtime and built-in
MCP tools for agent-assisted development.

**Primary CTA:** Try the demo

**Secondary CTA:** Build your first app

**Supporting line:** MIT-licensed · Go + TypeScript + PostgreSQL · Self-hosted

Use the demo CTA only when a working public demo exists. Until then, make the
quickstart primary and link to the example repository as secondary. Do not use
invented destinations, customer logos, testimonials, or benchmark numbers.

### Suggested narrative and supporting assets

| Content block | Message | Required evidence/asset |
| --- | --- | --- |
| Product introduction | What developers can build and who it is for | Hero copy and an actual Desk screenshot |
| From model to working screen | One DocType drives several standard application layers | A small real code excerpt beside the screen it produces |
| Business workflow | Rules, permissions, and reports participate in a complete flow | Short project/task walkthrough with observable outcomes |
| Agent-assisted development | MCP connects the agent to the framework's own tools and reference | Recorded metadata → change → migration → test sequence |
| Deployment | Binary, app files, and PostgreSQL form the base runtime | Verified standalone app setup and deployment guide |
| Trust and fit | Open source, documented contracts, visible roadmap | License, source, versioned docs, limitations and roadmap links |
| Next action | Move from observing to building | Working demo and tested quickstart |

Useful feature-card copy:

- **Model once. Start with a working interface.** Declare fields and relationships
  in TypeScript to generate standard forms, lists, and REST endpoints.
- **Put business rules on the server.** Validate documents and run lifecycle hooks
  through the framework's transactional document API.
- **Give your agent the framework's tools.** Inspect metadata, apply migrations,
  and run app tests through the built-in MCP server.
- **Deploy with a compact runtime architecture.** Run the ddcore binary with your
  app files and PostgreSQL, without a Node.js production server.

FAQ topics: What is a DocType? Do I need Go? Is Node required? How does MCP help?
Can I write custom business rules? What UI customization is supported? How is
ddcore related to Frappe? What needs validation before production?

Keep English as the source copy and provide Portuguese localization for outreach
to Brazilian developers. Technical terms and claims must remain equivalent across
languages. Avoid publishing an exact setup duration until user trials establish it.

## 5. Demo strategy

The sibling checkout `../ddcore-demo` was inspected locally because its public
GitHub content could not be retrieved during this review. Its README and source
layout cover projects, tasks, milestones, invoices/installments, settings,
extensions, reports, workspaces, and translations. Its Dockerfile uses the
published binary; no live deployment was inspected.

Use [ddcore-demo](https://github.com/jrvidotti/ddcore-demo) as the executable
tutorial. Keep `apps/testapp` as the framework's small acceptance fixture.
Preserve the example's teaching scope: future elaborate industry showcases belong
in separate apps instead of expanding the fixture or turning the tutorial into an ERP.

### First walkthrough: project operations

1. Open a seeded workspace containing understandable project/task data.
2. Open a project and explain its milestones and derived progress.
3. Start and complete a task; show the project's updated progress.
4. Attempt an invalid date change and show server-side validation.
5. Open the report and explain how it uses the same business data/services.
6. Inspect the relevant model, controller, and test in the repository.

Show the invoice/installment example as an additional lesson on monetary
precision. Do not suggest that it is a complete accounting or tax module.
Present any role-switching, approval, email, or webhook segment only after it has
been implemented and verified in the selected showcase; framework support alone
does not make it part of the current demo.

### Second walkthrough: develop with an agent

Use an isolated copy of the demo with a fixed starting commit. Ask the agent to
inspect a DocType, add a small business requirement and its test, migrate, and
demonstrate the result. Record the actual tools used, review the changes, and show
test output. Publish the resulting diff so viewers can reproduce the workflow.

### Public demo acceptance

- Published demo version matches its linked source and documentation.
- Fictional seed data, a documented reset process, and appropriate visitor roles.
- No visitor access to Administrator credentials or development MCP tools.
- Email and webhooks use controlled demo transports/destinations.
- A complete first walkthrough works from a fresh session.
- Loading and layout have been checked on desktop and mobile.
- Each showcased capability has a source-code link and an explanation.

Hosting, reset mechanics, and any demo changes are future work, not completed
deliverables of this document.

## 6. Documentation plan

Preserve `docs/agent/` as the canonical API reference. Build human learning paths
around it and publish the same reference content on the documentation site.
The landing page should summarize and link; it should not become a second API
specification that can diverge from the binary's embedded resources.

| Layer | Initial deliverables | Completion criterion |
| --- | --- | --- |
| Start | Install a pinned binary; create an app; configure PostgreSQL; migrate; open the first form | A new developer succeeds without cloning/building the framework |
| Understand | DocTypes, lifecycle, server/browser split, permissions, transactions, app boundaries | Every concept is tied to a small working example |
| Build | Project/task tutorial; validation; report; translations; testing | Tutorial code is runnable and matches a tagged example |
| Use an agent | MCP configuration, tools/resources, authority boundaries, worked change | Named tested clients can complete the documented flow |
| Reference | Publish the existing API pages with navigation, search, and release context | Site and embedded reference come from the same source |
| Operate | Deployment, optional mail/PDF dependencies, jobs, diagnostics, recovery procedures, upgrade limits | Instructions distinguish implemented tools from operator responsibilities |
| Evaluate | Feature/status matrix, limitations, benchmark method/results, roadmap | Readers can decide whether their application fits |

First documentation priorities:

1. Separate “build an app” from “contribute to the framework” in the README.
2. Add a verified first-app tutorial with explicit prerequisites and expected output.
3. Add a guided ddcore-demo tour with source links and screenshots.
4. Document one reproducible MCP development exercise.
5. Publish release-specific capability and deployment notes.

## 7. Distribution and rollout

Use a small pilot before broad promotion. Suggested roles below describe
responsibilities; staffing, budget, and dates remain unassigned. Phase exits are
more useful than calendar promises at this point.

| Phase | Suggested owner | Deliverables | Exit criterion |
| --- | --- | --- | --- |
| 1. Align evidence | Maintainer + content owner | Release-specific claim matrix, confirmed audience, proposed core message | Landing copy maps to evidence for the selected release |
| 2. Complete adoption path | Documentation + demo owners | First-app tutorial, reliable demo, source links | At least three external developers attempt the path; blocking issues are addressed |
| 3. Assemble launch assets | Landing agent + content owner | Integrated landing, short demo video, annotated article, screenshots | All destinations work and assets demonstrate the same app/version |
| 4. Pilot distribution | Maintainer/content owner | Targeted invitations and two technical walkthroughs | Feedback identifies successful activations and remaining friction |
| 5. Broaden distribution | Content/community owner | Release announcement, reusable tutorials, evidence-led follow-ups | Onboarding is repeatable and someone can respond to feedback |

Proposed content sequence:

1. **“From a TypeScript model to a business application”** — code, actual Desk,
   validation, and generated API; link to the first-app tutorial.
2. **“Building a ddcore feature with MCP”** — reproducible change with migration
   and tests; link to the agent guide.
3. **“Deploying a TypeScript business app with a Go runtime”** — binary/app/database
   boundaries and real setup; link to deployment documentation.
4. **“What ddcore shares with Frappe, and where it differs”** — honest model and
   compatibility discussion; link to an evaluation guide.
5. **“Measuring ddcore under a business workload”** — only after the benchmark
   artifact exists; link to raw methodology and results.

Start with the repository README/releases and the maintainer's existing audience.
Test outreach to TypeScript/business-software communities and agent-development
communities with different articles but one consistent product claim. Brazilian
developer channels can use Portuguese explanations and the localized demo.
Use Go-oriented channels for runtime architecture and measured engineering work.
Choose specific communities based on relevance and their current posting rules;
do not assume paid acquisition or a broad launch is required.

These are proposed channel experiments, not a completed market-demand study.
Publication, messages to third parties, hosting purchases, and campaign spending
require a separate execution decision.

## 8. Success measures and handoff

Optimize for developers reaching a working application, not only page views or
GitHub stars. The initial activation milestone is: **a developer creates a
DocType, migrates it, opens its form, and successfully tests a business rule.**

Track the website journey from content → landing → demo/quickstart. In the pilot,
observe time to first working form, setup failures, test completion, questions,
and return usage after a week. Use consented interviews or voluntary reporting
for local development outcomes; website clicks alone cannot measure CLI success.

Suggested pilot objectives, not adoption forecasts:

- Recruit five developers in the confirmed audience to attempt the first-app path.
- Observe at least three complete the activation milestone and record assistance.
- Identify and address the three most frequent onboarding obstacles.
- Obtain two reproducible example apps or substantive extension attempts.

The landing-page agent can use sections 1, 2, and 4 immediately as a content brief.
Demo and documentation owners can use sections 5 and 6 as scoped backlogs.
Numerical performance copy remains dependent on section 3's benchmark work.

Before public launch, confirm the target release, working demo and documentation
URLs, content language coverage, and who owns feedback/support.
No framework feature, demo behavior, landing implementation, or deployment was
changed as part of preparing this plan.
