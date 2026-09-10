# App Exemplo Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add the executable `exemplo` projects-and-tasks application, integrated with Cerne migration, desk, demo, tests, and acceptance coverage.

**Architecture:** Keep business invariants in synchronous DocType controllers and reusable server services. Controllers delegate project progress calculation to `services/projetos.ts`; scheduler, report, workspace, and desk consume the shared task-status summary service. The app is loaded from `cerne.json`, while the Go acceptance suite uses the checked-in app instead of generating a near-equivalent fixture.

**Tech Stack:** Go, PostgreSQL, TypeScript executed by goja, `@cerne/sdk`, `@cerne/desk-sdk`, Svelte desk bundle, Make.

**Spec:** `docs/superpowers/specs/2026-09-10-app-exemplo-design.md`

## Global Constraints

- Server-side app TypeScript is synchronous: never use `await` in controllers or services.
- Never edit generated `apps/exemplo/.cerne/types.d.ts`; regenerate it with `./bin/cerne types`.
- Do not use SQL directly in the app; reports and summaries use `cerne.db.getList`.
- Use translated English action/status keys in desk code and `translations/pt-BR.csv`.
- Run the relevant red/green test before proceeding and end with `make test`.

---

## File Structure

- `apps/exemplo/cerne.app.ts`: manifest, roles, scheduler, desk home/include.
- `apps/exemplo/doctypes/*`: declarative metadata, lifecycle rules, desk scripts, and behavior tests.
- `apps/exemplo/services/projetos.ts`: only project progress recalculation.
- `apps/exemplo/services/tarefas.ts`: overdue transitions and reusable grouped status summary.
- `apps/exemplo/services/demo.ts`: idempotent sample-data installation.
- `apps/exemplo/reports` and `workspaces`: read-only dashboard/report adapters around the service.
- `internal/acceptance/acceptance_test.go`: boots the actual checked-in app and validates installation/desk API behavior.
- `cerne.json` and `Makefile`: load and type-check/test the new app.

### Task 1: App bootstrap and declarative schema

**Files:**
- Create: `apps/exemplo/cerne.app.ts`, `apps/exemplo/tsconfig.json`, `apps/exemplo/translations/pt-BR.csv`
- Create: `apps/exemplo/doctypes/marco_projeto/marco_projeto.doctype.ts`
- Create: `apps/exemplo/doctypes/projeto/projeto.doctype.ts`
- Create: `apps/exemplo/doctypes/tarefa/tarefa.doctype.ts`
- Modify: `cerne.json`, `Makefile`, `.gitignore` only if `.cerne/` is not already ignored

**Produces:** `Marco Projeto`, `Projeto`, and `Tarefa` metadata; manifest with roles `Gestor de Projetos` / `Colaborador de Projetos`, `desk.home: "Projetos"`, global list include, and `daily: ["exemplo.services.tarefas.marcarAtrasadas"]`.

- [ ] **Step 1: Write the failing metadata compilation check**

Run: `./bin/cerne types`

Expected: FAIL because `cerne.json` does not yet reference `apps/exemplo` and its app files do not exist.

- [ ] **Step 2: Add the minimal app manifest and schemas**

Implement the exact field contracts from the spec. In particular, use `isChild: true` and `{ fieldname: "concluido_em", fieldtype: "Date", mandatoryDependsOn: "doc.concluido" }` for `Marco Projeto`; use `{ naming: { field: "codigo" }, titleField: "titulo", trackChanges: true }` in both main DocTypes; make derived fields (`status`, `progresso`, `concluida_em`) `readOnly: true`; and define permissions:

```ts
{ role: "Gestor de Projetos", read: true, write: true, create: true, delete: true, report: true, export: true }
{ role: "Colaborador de Projetos", read: true } // Projeto
{ role: "Colaborador de Projetos", read: true, write: true, create: true, report: true } // Tarefa
```

Set `cerne.json` apps to `["apps/exemplo"]`. Extend `check` to invoke `./bin/cerne types` followed by `npx tsc -p apps/exemplo/tsconfig.json --noEmit`, and extend `test` to call `./bin/cerne test --app exemplo` after Go tests.

- [ ] **Step 3: Regenerate types and verify compilation**

Run: `./bin/cerne types && make check`

Expected: types appear only under ignored `apps/exemplo/.cerne/` and both desk and app type checks pass.

- [ ] **Step 4: Commit**

```bash
git add cerne.json Makefile apps/exemplo .gitignore
git commit -m "feat: scaffold example projects app"
```

### Task 2: Project lifecycle and progress service

**Files:**
- Create: `apps/exemplo/services/projetos.ts`
- Create: `apps/exemplo/doctypes/projeto/projeto.controller.ts`
- Create: `apps/exemplo/doctypes/projeto/projeto.test.ts`

**Consumes:** the schema from Task 1 and `cerne.db.getList`, `Document.dbSet`, `Document.getDocBeforeSave`.

**Produces:** `recalcularProgresso(projeto: string): void`, validation of date range/milestone dates, and project status/progress derived from tasks.

- [ ] **Step 1: Write failing controller/service tests**

Create tests that prove: a project rejects `data_final < data_inicio`; saving a closed milestone without its date fails and reopening a milestone clears `concluido_em`; no tasks gives `{ progresso: 0, status: "Planejado" }`; one of two completed tasks gives `{ progresso: 50, status: "Em andamento" }`; two of two gives `{ progresso: 100, status: "Concluído" }`.

```ts
expect(() => cerne.newDoc("Projeto", { codigo: "P-1", titulo: "P", responsavel: "Administrator", data_inicio: "2026-01-02", data_final: "2026-01-01" }).insert()).toThrow("final");
```

- [ ] **Step 2: Verify tests fail for the intended missing behavior**

Run: `./bin/cerne test --app exemplo --filter 'Projeto'`

Expected: FAIL on the invalid interval and derived values.

- [ ] **Step 3: Implement only the lifecycle and calculation behavior**

Use `validate(doc)` to compare civil date strings and normalize milestone consistency. Implement:

```ts
export function recalcularProgresso(projeto: string): void {
  const tarefas = cerne.db.getList("Tarefa", { filters: { projeto }, fields: ["status"], limit: 10000 });
  const concluidas = tarefas.filter(t => t.status === "Concluída").length;
  const status = tarefas.length === 0 ? "Planejado" : concluidas === tarefas.length ? "Concluído" : "Em andamento";
  const progresso = tarefas.length === 0 ? 0 : cerne.utils.roundTo(concluidas * 100 / tarefas.length, 2);
  cerne.getDoc("Projeto", projeto).dbSet({ progresso, status });
}
```

- [ ] **Step 4: Verify green**

Run: `./bin/cerne test --app exemplo --filter 'Projeto'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/exemplo/services/projetos.ts apps/exemplo/doctypes/projeto
git commit -m "feat: derive example project progress"
```

### Task 3: Task lifecycle, parent recalculation, and overdue service

**Files:**
- Create: `apps/exemplo/doctypes/tarefa/tarefa.controller.ts`
- Create: `apps/exemplo/services/tarefas.ts`
- Create: `apps/exemplo/doctypes/tarefa/tarefa.test.ts`
- Create: `apps/exemplo/services/tarefas.test.ts`

**Consumes:** `recalcularProgresso(projeto)` and Tarefa metadata.

**Produces:** controller methods `iniciar`, `concluir`, `reabrir`; `marcarAtrasadas(): number`; whitelisted `marcarAtrasadasAgora`; `resumoPorStatus(filters)`.

- [ ] **Step 1: Write failing task and service tests**

Cover defaults and constraints: insert starts `Aberta`; a deadline before its project start is rejected; start/complete/reopen are idempotent and return `{ status, concluida_em }`; reopen a past-deadline task yields `Atrasada`; insert/update/delete recalculates the project, including both old and new project when `projeto` changes; scheduler only changes past-due `Aberta`/`Em andamento`, preserves `Concluída`, logs isolated failures, and reports its count.

```ts
const result = tarefa.runMethod("concluir");
expect(result.status).toBe("Concluída");
expect(tarefa.runMethod("concluir").status).toBe("Concluída");
```

- [ ] **Step 2: Verify red**

Run: `./bin/cerne test --app exemplo --filter 'Tarefa|atrasadas|dois projetos'`

Expected: FAIL because methods/services are missing.

- [ ] **Step 3: Implement transitions and hooks synchronously**

In `validate`, reject deadline earlier than `cerne.db.getValue("Projeto", doc.projeto, "data_inicio")`; in `beforeInsert`, make `status = "Aberta"`; in `afterInsert`, `onUpdate`, and `afterDelete`, call the project service. For a move, take the old `projeto` from `getDocBeforeSave()` and recalculate it if distinct. Methods set fields, call `save()`, and return only `{ status: doc.status, concluida_em: doc.concluida_em }`.

In `marcarAtrasadas`, query with filters equivalent to deadline `< cerne.utils.today()` and status `in ["Aberta", "Em andamento"]`; load/save each doc inside `try/catch`, call `cerne.log.error` in the catch, increment after save. Export `marcarAtrasadasAgora = whitelisted(() => marcarAtrasadas(), { roles: ["Gestor de Projetos"] })`. Build `resumoPorStatus` from `getList` and return all four statuses with `quantidade` and a two-decimal percentage.

- [ ] **Step 4: Verify green**

Run: `./bin/cerne test --app exemplo --filter 'Tarefa|atrasadas|dois projetos'`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/exemplo/doctypes/tarefa apps/exemplo/services
git commit -m "feat: add example task workflow"
```

### Task 4: Demo data, report, workspace, forms, list view, and translations

**Files:**
- Create: `apps/exemplo/services/demo.ts`, `apps/exemplo/services/demo.test.ts`
- Create: `apps/exemplo/reports/tarefas_por_status.report.ts`
- Create: `apps/exemplo/workspaces/projetos.workspace.ts`
- Create: `apps/exemplo/doctypes/projeto/projeto.form.ts`, `apps/exemplo/doctypes/tarefa/tarefa.form.ts`, `apps/exemplo/client/listas.ts`
- Modify: `apps/exemplo/translations/pt-BR.csv`

**Consumes:** task methods, `resumoPorStatus(filters)`, app manifest includes.

**Produces:** idempotent `gerar()`, report/workspace UI, and desk actions that never write derived fields directly.

- [ ] **Step 1: Write failing demo/report tests**

Test `gerar()` twice: first response has project `DEMO`, three milestones and three task names; second adds zero records. Test report filters (`projeto`, `responsavel`, `data_limite_ate`) and totals/percentages, including its bar chart labels and values.

- [ ] **Step 2: Verify red**

Run: `./bin/cerne test --app exemplo --filter 'demo|Relatório|status'`

Expected: FAIL because demo/report modules do not exist.

- [ ] **Step 3: Implement demo and read-only adapters**

`gerar()` first checks each code with `cerne.db.exists`, creates `DEMO` with three relative-date milestones, inserts `DEMO-01` through `DEMO-03`, then calls controller methods to obtain in-progress/completed states. Return `{ criados: string[], quantidade: number }`.

The report calls `resumoPorStatus(filters)` and returns columns `status`, `quantidade`, `percentual` plus a bar `chart`. Workspace exposes explicit sidebar links, shortcuts, cards for ongoing/open/overdue entities, grouped `Planejamento`/`Acompanhamento` links, and a chart whose method calls the shared summary. Forms use `frm.call("iniciar" | "concluir" | "reabrir")`, then `frm.reload()`; task setup filters project status to non-completed; list view orders `data_limite asc` and maps the four statuses to blue/yellow/red/green indicators. Translate all action/status keys required by the spec.

- [ ] **Step 4: Verify green and compile desk bundles**

Run: `./bin/cerne test --app exemplo && make check`

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add apps/exemplo
git commit -m "feat: add example desk dashboard and demo"
```

### Task 5: Replace the temporary acceptance fixture with the real app

**Files:**
- Modify: `internal/acceptance/acceptance_test.go`

**Consumes:** checked-in `apps/exemplo` and its metadata/translation/workspace.

**Produces:** acceptance checks that migrate the real example app on a disposable database, validate idempotent plan, boot home/sidebar destinations, and translations without generating a temporary product-like app.

- [ ] **Step 1: Write the failing acceptance expectation**

Replace `acceptanceApp(t)` use in setup with a repository app resolver (`filepath.Abs("../../apps/exemplo")` from `internal/acceptance`). Change expectations to `Gestor de Projetos`, `Projetos`, `Projeto`, `Tarefa`, and `Tarefas por Status`, then add a test invoking `exemplo.services.demo.gerar` twice through `RunJob` and checking no duplicate `DEMO` documents.

- [ ] **Step 2: Verify red**

Run: `go test ./internal/acceptance -run 'Test(Instalacao|BootHome|Traducoes|Demo)' -count=1`

Expected: FAIL until the acceptance setup receives the real app and assertions match its public metadata.

- [ ] **Step 3: Implement the smallest acceptance refactor**

Delete the temp-directory app generator and append `js.App{Name: "exemplo", Dir: appDir}` in setup. Keep the disposable database lifecycle and HTTP boot assertions. Verify translations include a specified app key such as `Start -> Iniciar` and sidebar targets all exist in boot data.

- [ ] **Step 4: Verify green**

Run: `go test ./internal/acceptance -count=1`

Expected: PASS (or explicit skip only when Postgres is unavailable, as the existing suite defines).

- [ ] **Step 5: Commit**

```bash
git add internal/acceptance/acceptance_test.go
git commit -m "test: accept the checked-in example app"
```

### Task 6: End-to-end verification and generated types hygiene

**Files:**
- Modify only if verification exposes a scoped defect in files from Tasks 1-5.

- [ ] **Step 1: Validate metadata and migration idempotence**

Run: `./bin/cerne migrate --dry-run`

Expected: the first clean-database migration creates the example schema; the immediate subsequent dry run contains no DDL.

- [ ] **Step 2: Run app demo idempotence against the configured dev database**

Run: `./bin/cerne demo --app exemplo && ./bin/cerne demo --app exemplo`

Expected: the second result reports zero new records.

- [ ] **Step 3: Run the complete repository verification**

Run: `make test`

Expected: build, vet, all Go suites, desk checks/tests, and `./bin/cerne test --app exemplo` exit zero.

- [ ] **Step 4: Inspect final scope**

Run: `git status --short && git diff --check`

Expected: only planned source/config/test changes are tracked, generated `.cerne/` remains ignored, and no whitespace errors appear.

