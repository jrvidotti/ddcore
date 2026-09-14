# Workspace Selector & URL Prefix Routing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement a workspace selector in the Desk sidebar and prefix Desk URLs with the active workspace (`/app/[workspace]/[doctype]`), filtering navigation items by active workspace while maintaining backward compatibility for legacy un-prefixed routes.

**Architecture:** A pure helper module (`sidebar-workspace.ts`) manages workspace resolution, URL generation, and `localStorage` persistence with 100% unit test coverage. SvelteKit routing under `desk/src/routes/app/[workspace]` handles workspace dashboards, DocType lists, form views, and reports, while redirecting short/legacy routes (`/app/[doctype]` or `/app`) to their proper workspace destination. `Sidebar.svelte` renders the interactive switcher and active workspace items.

**Tech Stack:** Svelte 5 (runes: `$state`, `$derived`, `$effect`), SvelteKit 2, TypeScript, Vitest.

**Spec:** [`docs/superpowers/specs/2026-09-14-workspace-selector-design.md`](file:///Users/junior/dev/ddcore/docs/superpowers/specs/2026-09-14-workspace-selector-design.md)

## Global Constraints

- Svelte 5 runes (`$state`, `$derived`, `$effect`, `$props`) must be used for Desk reactivity.
- Server TypeScript in Goja is not affected (this is purely desk frontend code).
- User-facing text must use English as canonical string and `__("...")` for translation.
- No SvelteKit route collisions: cannot have sibling parameter directories like `[workspace]` and `[doctype]` at `desk/src/routes/app/`.
- All tests must pass: `npm --prefix desk test` and `npm --prefix desk run check`.

---

### Task 1: Workspace Resolution Logic & Unit Tests (`sidebar-workspace.ts`)

**Files:**
- Create: `desk/src/lib/components/sidebar-workspace.ts`
- Test: `desk/src/lib/components/sidebar-workspace.test.ts`

**Interfaces:**
- Consumes: `Boot` data shape (`workspaces`, `doctypes`) from `$lib/boot.svelte`.
- Produces:
  - `export interface WorkspaceItem`
  - `export function resolveActiveWorkspace(params: { currentPath: string; workspaces: WorkspaceItem[]; doctypes?: Record<string, { app?: string }>; remembered?: string }): WorkspaceItem | null`
  - `export function resolveWorkspaceForDoctype(doctype: string, workspaces: WorkspaceItem[], doctypesMeta?: Record<string, { app?: string }>): string | null`
  - `export function workspaceItemHref(workspaceName: string, item: { route?: string; doctype?: string; report?: string }): string`
  - `export function rememberWorkspace(name: string): void`
  - `export function getRememberedWorkspace(): string`

- [ ] **Step 1: Write the failing unit tests**

Create `desk/src/lib/components/sidebar-workspace.test.ts`:
```ts
import { describe, it, expect, beforeEach } from "vitest";
import {
  resolveActiveWorkspace,
  resolveWorkspaceForDoctype,
  workspaceItemHref,
  rememberWorkspace,
  getRememberedWorkspace,
  type WorkspaceItem,
} from "./sidebar-workspace";

const mockWorkspaces: WorkspaceItem[] = [
  {
    name: "Alugueis",
    label: "Aluguéis",
    icon: "building-2",
    app: "alugueis",
    sidebar: [
      { label: "Visão Geral", route: "/app/Alugueis", icon: "layout-dashboard" },
      { label: "Contratos", doctype: "Contrato", icon: "notepad-text" },
      { label: "Imóveis", doctype: "Imovel", icon: "building-2" },
      { label: "Relatórios", icon: "bar-chart-3" },
      { label: "Contratos a Vencer", report: "Contratos a Vencer" },
    ],
  },
  {
    name: "Manutencao",
    label: "Maintenance",
    icon: "settings",
    app: "manutencao",
    sidebar: [
      { label: "Overview", route: "/app/Manutencao", icon: "layout-dashboard" },
      { label: "Ordens de Serviço", doctype: "OrdemServico", icon: "wrench" },
    ],
  },
];

const mockDoctypes = {
  Contrato: { app: "alugueis" },
  Imovel: { app: "alugueis" },
  OrdemServico: { app: "manutencao" },
  User: { app: "core" },
};

describe("sidebar-workspace", () => {
  it("resolves active workspace from direct workspace route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Manutencao",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("resolves active workspace from prefixed doctype route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Alugueis/Contrato",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("resolves active workspace from prefixed document form route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Alugueis/Contrato/CTR-0001",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("resolves active workspace for legacy un-prefixed doctype route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/OrdemServico",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("resolves active workspace for legacy report route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/report/Contratos%20a%20Vencer",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("falls back to remembered workspace on global routes like /app/notifications", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/notifications",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
      remembered: "Manutencao",
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("falls back to first workspace if nothing is remembered", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/unknown",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("returns null when workspaces list is empty", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/notifications",
      workspaces: [],
      doctypes: mockDoctypes,
    });
    expect(ws).toBeNull();
  });

  it("resolves owning workspace for a doctype", () => {
    expect(resolveWorkspaceForDoctype("Contrato", mockWorkspaces, mockDoctypes)).toBe("Alugueis");
    expect(resolveWorkspaceForDoctype("OrdemServico", mockWorkspaces, mockDoctypes)).toBe("Manutencao");
    expect(resolveWorkspaceForDoctype("NonExistent", mockWorkspaces, mockDoctypes)).toBeNull();
  });

  it("generates correct workspace prefixed hrefs", () => {
    expect(workspaceItemHref("Alugueis", { doctype: "Contrato" })).toBe("/app/Alugueis/Contrato");
    expect(workspaceItemHref("Alugueis", { report: "Contratos a Vencer" })).toBe("/app/Alugueis/report/Contratos%20a%20Vencer");
    expect(workspaceItemHref("Alugueis", { route: "/app/Alugueis" })).toBe("/app/Alugueis");
    expect(workspaceItemHref("Alugueis", { label: "Group header" })).toBe("");
  });

  it("remembers and retrieves workspace in storage safely", () => {
    rememberWorkspace("Manutencao");
    expect(getRememberedWorkspace()).toBe("Manutencao");
  });
});
```

- [ ] **Step 2: Run test to verify it fails**

Run: `npm --prefix desk test src/lib/components/sidebar-workspace.test.ts`  
Expected: FAIL (Cannot find module `./sidebar-workspace`)

- [ ] **Step 3: Write implementation**

Create `desk/src/lib/components/sidebar-workspace.ts`:
```ts
export interface WorkspaceItem {
  name: string;
  label: string;
  icon?: string;
  app?: string;
  roles?: string[];
  sidebar?: Array<{
    label: string;
    doctype?: string;
    report?: string;
    route?: string;
    icon?: string;
  }>;
}

const WORKSPACE_KEY = "ddcore_workspace";

export function rememberWorkspace(name: string): void {
  try {
    if (typeof localStorage !== "undefined" && name) {
      localStorage.setItem(WORKSPACE_KEY, name);
    }
  } catch {
    /* private mode / SSR */
  }
}

export function getRememberedWorkspace(): string {
  try {
    if (typeof localStorage !== "undefined") {
      return localStorage.getItem(WORKSPACE_KEY) || "";
    }
  } catch {
    /* private mode / SSR */
  }
  return "";
}

/**
 * Resolves which workspace owns a given DocType:
 * 1. Workspace that explicitly declares `doctype` in its sidebar.
 * 2. Workspace that belongs to the same app as the DocType.
 */
export function resolveWorkspaceForDoctype(
  doctype: string,
  workspaces: WorkspaceItem[],
  doctypesMeta?: Record<string, { app?: string }>,
): string | null {
  if (!doctype || !workspaces.length) return null;

  // 1. Direct match in workspace sidebar
  for (const ws of workspaces) {
    if (ws.sidebar?.some((item) => item.doctype === doctype)) {
      return ws.name;
    }
  }

  // 2. App-level match
  const dtApp = doctypesMeta?.[doctype]?.app;
  if (dtApp) {
    const ws = workspaces.find((w) => w.app === dtApp);
    if (ws) return ws.name;
  }

  return null;
}

/**
 * Resolves which workspace owns a given report name.
 */
export function resolveWorkspaceForReport(
  reportName: string,
  workspaces: WorkspaceItem[],
): string | null {
  if (!reportName || !workspaces.length) return null;
  for (const ws of workspaces) {
    if (ws.sidebar?.some((item) => item.report === reportName)) {
      return ws.name;
    }
  }
  return null;
}

/**
 * Generates the prefixed URL for a sidebar item.
 */
export function workspaceItemHref(
  workspaceName: string,
  item: { route?: string; doctype?: string; report?: string },
): string {
  if (item.route) return item.route;
  if (item.doctype) {
    return `/app/${encodeURIComponent(workspaceName)}/${encodeURIComponent(item.doctype)}`;
  }
  if (item.report) {
    return `/app/${encodeURIComponent(workspaceName)}/report/${encodeURIComponent(item.report)}`;
  }
  return "";
}

/**
 * Resolves the active workspace from the current pathname, matching doctypes,
 * remembered workspace, or default fallback.
 */
export function resolveActiveWorkspace(params: {
  currentPath: string;
  workspaces: WorkspaceItem[];
  doctypes?: Record<string, { app?: string }>;
  remembered?: string;
}): WorkspaceItem | null {
  const { currentPath, workspaces, doctypes, remembered } = params;
  if (!workspaces || workspaces.length === 0) return null;

  // Normalize path parts: /app/:part1/:part2...
  const segments = currentPath.replace(/^\/+|\/+$/g, "").split("/");
  if (segments[0] === "app" && segments.length >= 2) {
    const part1 = decodeURIComponent(segments[1]);

    // 1. Does part1 match a known workspace name?
    const directWs = workspaces.find((w) => w.name.toLowerCase() === part1.toLowerCase());
    if (directWs) return directWs;

    // 2. Is it /app/workspace/:name (legacy workspace route)?
    if (part1 === "workspace" && segments[2]) {
      const wsName = decodeURIComponent(segments[2]);
      const legacyWs = workspaces.find((w) => w.name.toLowerCase() === wsName.toLowerCase());
      if (legacyWs) return legacyWs;
    }

    // 3. Is it /app/report/:reportName (legacy report route)?
    if (part1 === "report" && segments[2]) {
      const repName = decodeURIComponent(segments[2]);
      const repWs = resolveWorkspaceForReport(repName, workspaces);
      if (repWs) {
        const found = workspaces.find((w) => w.name === repWs);
        if (found) return found;
      }
    }

    // 4. Is part1 a legacy un-prefixed DocType (e.g. /app/Contrato)?
    const dtWsName = resolveWorkspaceForDoctype(part1, workspaces, doctypes);
    if (dtWsName) {
      const found = workspaces.find((w) => w.name === dtWsName);
      if (found) return found;
    }
  }

  // 5. Fall back to remembered workspace
  if (remembered) {
    const remWs = workspaces.find((w) => w.name.toLowerCase() === remembered.toLowerCase());
    if (remWs) return remWs;
  }

  // 6. Default to first workspace
  return workspaces[0];
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `npm --prefix desk test src/lib/components/sidebar-workspace.test.ts`  
Expected: PASS (all tests passing)

- [ ] **Step 5: Commit**

```bash
git add desk/src/lib/components/sidebar-workspace.ts desk/src/lib/components/sidebar-workspace.test.ts
git commit -m "feat(desk): add workspace resolution logic and unit tests"
```

---

### Task 2: Dynamic SvelteKit Workspace & DocType Routes

**Files:**
- Create: `desk/src/routes/app/[workspace]/+page.svelte`
- Create: `desk/src/routes/app/[workspace]/[doctype]/+page.svelte`
- Create: `desk/src/routes/app/[workspace]/[doctype]/[name]/+page.svelte`
- Create: `desk/src/routes/app/[workspace]/[doctype]/new/+page.svelte`
- Create: `desk/src/routes/app/[workspace]/report/[name]/+page.svelte`
- Delete: `desk/src/routes/app/[doctype]` directory (to eliminate sibling route conflict)
- Modify: `desk/src/routes/app/+page.svelte`
- Modify: `desk/src/routes/app/workspace/[name]/+page.svelte`

- [ ] **Step 1: Create `desk/src/routes/app/[workspace]/+page.svelte`**

Handles workspace dashboard rendering OR backward-compatible redirect when `[workspace]` is actually a DocType name:
```svelte
<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot } from "$lib/boot.svelte";
  import { resolveWorkspaceForDoctype } from "$lib/components/sidebar-workspace";
  import Workspace from "$lib/components/Workspace.svelte";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";
  import { getMeta } from "$lib/meta";

  const segment = $derived(page.params.workspace ?? "");
  const ws = $derived(boot.data?.workspaces?.find((w: any) => w.name.toLowerCase() === segment.toLowerCase()));
  const isDoctype = $derived(boot.data?.doctypes && segment in boot.data.doctypes);

  $effect(() => {
    if (boot.ready && !ws && isDoctype) {
      const realWs = resolveWorkspaceForDoctype(segment, boot.data?.workspaces || [], boot.data?.doctypes);
      if (realWs) {
        goto(`/app/${encodeURIComponent(realWs)}/${encodeURIComponent(segment)}`, { replaceState: true });
      }
    }
  });
</script>

{#if ws}
  {#key ws.name}
    <Workspace name={ws.name} />
  {/key}
{:else if isDoctype}
  {#key segment}
    {#await getMeta(segment) then m}
      {#if m.doctype.isSingle}
        <FormView doctype={segment} name="singleton" />
      {:else}
        <ListView doctype={segment} />
      {/if}
    {:catch error}
      <div class="page"><div class="card empty">{error.message}</div></div>
    {/await}
  {/key}
{:else if boot.ready}
  <div class="page"><div class="card empty">Workspace "{segment}" not found.</div></div>
{/if}
```

- [ ] **Step 2: Create `desk/src/routes/app/[workspace]/[doctype]/+page.svelte`**

Handles DocType list view rendering OR backward-compatible redirect when `/app/[doctype]/[name]` landed here:
```svelte
<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot } from "$lib/boot.svelte";
  import { resolveWorkspaceForDoctype } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";

  const workspace = $derived(page.params.workspace ?? "");
  const doctype = $derived(page.params.doctype ?? "");

  const isWorkspace = $derived(boot.data?.workspaces?.some((w: any) => w.name.toLowerCase() === workspace.toLowerCase()));
  const isDocInFirstPos = $derived(boot.data?.doctypes && workspace in boot.data.doctypes);

  $effect(() => {
    if (boot.ready && !isWorkspace && isDocInFirstPos) {
      const realWs = resolveWorkspaceForDoctype(workspace, boot.data?.workspaces || [], boot.data?.doctypes);
      if (realWs) {
        goto(`/app/${encodeURIComponent(realWs)}/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}`, { replaceState: true });
      }
    }
  });

  const meta = $derived(getMeta(doctype));
</script>

{#key doctype}
  {#await meta then m}
    {#if m.doctype.isSingle}
      <FormView {doctype} name="singleton" />
    {:else}
      <ListView {doctype} />
    {/if}
  {:catch error}
    <div class="page"><div class="card empty">{error.message}</div></div>
  {/await}
{/key}
```

- [ ] **Step 3: Create `desk/src/routes/app/[workspace]/[doctype]/new/+page.svelte`**

```svelte
<script lang="ts">
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
</script>
{#key doctype}
  <FormView {doctype} name="new" />
{/key}
```

- [ ] **Step 4: Create `desk/src/routes/app/[workspace]/[doctype]/[name]/+page.svelte`**

```svelte
<script lang="ts">
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
  const name = $derived(page.params.name ?? "");
</script>
{#key doctype + "/" + name}
  <FormView {doctype} {name} />
{/key}
```

- [ ] **Step 5: Create `desk/src/routes/app/[workspace]/report/[name]/+page.svelte`**

```svelte
<script lang="ts">
  import { page } from "$app/state";
  import ReportView from "$lib/components/ReportView.svelte";
  const name = $derived(page.params.name ?? "");
</script>
{#key name}<ReportView {name} />{/key}
```

- [ ] **Step 6: Remove legacy `desk/src/routes/app/[doctype]` directory**

Delete `desk/src/routes/app/[doctype]` (all files inside: `+page.svelte`, `[name]/+page.svelte`, `new/+page.svelte`) using `git rm -r desk/src/routes/app/[doctype]`.

- [ ] **Step 7: Update `desk/src/routes/app/+page.svelte` and `desk/src/routes/app/workspace/[name]/+page.svelte`**

In `desk/src/routes/app/+page.svelte`:
Redirect root `/app` to `/app/[workspace]`:
```svelte
<script lang="ts">
  import { boot, __ } from "$lib/boot.svelte";
  import { goto } from "$app/navigation";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";

  $effect(() => {
    if (boot.ready) {
      const rem = getRememberedWorkspace();
      const target = boot.data?.workspaces?.find((w: any) => w.name.toLowerCase() === rem.toLowerCase())?.name ||
        boot.data?.apps?.map((a: any) => a.desk?.home).find(Boolean) ||
        boot.data?.workspaces?.[0]?.name;
      if (target) {
        goto(`/app/${encodeURIComponent(target)}`, { replaceState: true });
      }
    }
  });
</script>

{#if boot.ready && !boot.data?.workspaces?.length}
  <div class="page"><div class="card empty">{__("No workspace declared.")}</div></div>
{:else}
  <div class="page muted">…</div>
{/if}
```

In `desk/src/routes/app/workspace/[name]/+page.svelte`:
```svelte
<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  const name = $derived(page.params.name ?? "");
  $effect(() => {
    if (name) goto(`/app/${encodeURIComponent(name)}`, { replaceState: true });
  });
</script>
```

- [ ] **Step 8: Run check to verify no SvelteKit route collisions or errors**

Run: `npm --prefix desk run check`  
Expected: PASS with 0 errors.

- [ ] **Step 9: Commit**

```bash
git add desk/src/routes/app
git commit -m "feat(desk): restructure routes under /app/[workspace]"
```

---

### Task 3: Sidebar Workspace Selector & Filtered Navigation (`Sidebar.svelte`)

**Files:**
- Modify: `desk/src/lib/components/Sidebar.svelte`

- [ ] **Step 1: Add workspace selector and active workspace derivation to `Sidebar.svelte`**

Import helper functions from `./sidebar-workspace`:
```ts
import {
  resolveActiveWorkspace,
  rememberWorkspace,
  getRememberedWorkspace,
  workspaceItemHref,
  type WorkspaceItem,
} from "./sidebar-workspace";
```

Derive active workspace:
```ts
const workspaces = $derived((boot.data?.workspaces || []) as WorkspaceItem[]);
let remembered = $state(getRememberedWorkspace());
const activeWorkspace = $derived(resolveActiveWorkspace({
  currentPath: current,
  workspaces,
  doctypes: boot.data?.doctypes,
  remembered,
}));

$effect(() => {
  if (activeWorkspace?.name) {
    remembered = activeWorkspace.name;
    rememberWorkspace(activeWorkspace.name);
  }
});

let wsMenuOpen = $state(false);

function selectWorkspace(name: string) {
  wsMenuOpen = false;
  rememberWorkspace(name);
  remembered = name;
  goto(`/app/${encodeURIComponent(name)}`);
}
```

Update outside click handler:
```ts
function onPointerDown(e: PointerEvent) {
  const target = e.target as HTMLElement | null;
  if (menuOpen && !target?.closest(".foot .dropdown")) menuOpen = false;
  if (wsMenuOpen && !target?.closest(".workspace-switcher")) wsMenuOpen = false;
}
function onKeydown(e: KeyboardEvent) {
  if (e.key === "Escape") {
    menuOpen = false;
    wsMenuOpen = false;
  }
}
```

- [ ] **Step 2: Update HTML markup in `Sidebar.svelte`**

Place the workspace selector below `<div class="brand">`:
```svelte
{#if workspaces.length > 1}
  <div class="workspace-switcher dropdown">
    <button class="workspace-btn" onclick={() => (wsMenuOpen = !wsMenuOpen)} aria-haspopup="menu" aria-expanded={wsMenuOpen}>
      <span class="ws-icon"><Icon name={activeWorkspace?.icon || "layout-dashboard"} size={16} /></span>
      <span class="ws-label">{activeWorkspace?.label || activeWorkspace?.name}</span>
      <Icon name={wsMenuOpen ? "chevron-up" : "chevron-down"} size={13} />
    </button>
    {#if wsMenuOpen}
      <div class="menu" role="menu">
        {#each workspaces as ws}
          <button
            role="menuitem"
            class:selected={ws.name === activeWorkspace?.name}
            onclick={() => selectWorkspace(ws.name)}
          >
            <Icon name={ws.icon || "layout-dashboard"} size={14} />
            <span class="ws-option-label">{ws.label || ws.name}</span>
            {#if ws.name === activeWorkspace?.name}
              <Icon name="check" size={14} class="ws-check" />
            {/if}
          </button>
        {/each}
      </div>
    {/if}
  </div>
{:else if workspaces.length === 1}
  <div class="workspace-static-header">
    <Icon name={workspaces[0].icon || "layout-dashboard"} size={16} />
    <span>{workspaces[0].label || workspaces[0].name}</span>
  </div>
{/if}
```

Update `<nav>` items to render ONLY `activeWorkspace.sidebar`:
```svelte
<nav>
  <a href="/app/notifications" class:active={active("/app/notifications")}>
    <Icon name="bell" /><span>{__("Notifications")}</span>
    {#if notifications.unread > 0}<span class="notification-count" aria-label={__("{0} unread notifications", [notifications.unread])}>{notifications.unread}</span>{/if}
  </a>
  <a href="/app/todo" class:active={active("/app/todo")}>
    <Icon name="check-square" /><span>{__("To-Do")}</span>
    {#if pendingTasks.count > 0}<span class="notification-count" aria-label={__("{0} pending tasks", [pendingTasks.count])}>{pendingTasks.count}</span>{/if}
  </a>
  {#if activeWorkspace}
    {#each activeWorkspace.sidebar || [] as it}
      {#if itemHref(it)}
        <a href={itemHref(it)} class:active={active(itemHref(it))} class:child={!it.icon}><Icon name={it.icon || "circle"} size={it.icon ? 16 : 6} /><span>{it.label}</span></a>
      {:else}
        <div class="group">{it.label}</div>
      {/if}
    {/each}
  {/if}
  {#if otherDoctypes.length}
    <div class="group" style="cursor:pointer" onclick={() => (showCore = !showCore)} role="button" tabindex="0" onkeydown={(e) => e.key === "Enter" && (showCore = !showCore)}>
      {__("System")} <Icon name={showCore ? "chevron-down" : "chevron-right"} size={12} />
    </div>
    {#if showCore}
      {#each otherDoctypes as [name, d]}
        {@const coreHref = `/app/${encodeURIComponent(activeWorkspace?.name || "core")}/${encodeURIComponent(name)}`}
        <a href={coreHref} class:active={active(coreHref)}><Icon name={d.icon || "circle"} size={d.icon ? 16 : 6} /><span>{d.label}</span></a>
      {/each}
    {/if}
  {/if}
</nav>
```

And update `itemHref`:
```ts
const itemHref = (it: any) => workspaceItemHref(activeWorkspace?.name || "", it);
```

- [ ] **Step 3: Add CSS styles for `.workspace-switcher`**

In `<style>`:
```css
.workspace-switcher { padding: 8px 10px; border-bottom: 1px solid var(--border); }
.workspace-btn { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; border: 1px solid var(--border); background: #fafafa; border-radius: 6px; cursor: pointer; text-align: left; font-size: 13px; font-weight: 500; color: var(--text); }
.workspace-btn:hover { background: #f3f4f6; }
.workspace-btn .ws-icon { display: flex; align-items: center; color: var(--primary); }
.workspace-btn .ws-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.workspace-switcher .menu { width: calc(100% - 20px); left: 10px; right: 10px; }
.workspace-switcher .menu button { display: flex; align-items: center; gap: 8px; }
.workspace-switcher .menu button.selected { background: #eff6ff; color: var(--primary); font-weight: 500; }
.workspace-switcher .menu .ws-option-label { flex: 1; text-align: left; }
.workspace-static-header { display: flex; align-items: center; gap: 8px; padding: 12px 16px 6px; font-size: 13px; font-weight: 600; color: var(--text); }
```

- [ ] **Step 4: Run tests and type check**

Run: `npm --prefix desk test && npm --prefix desk run check`  
Expected: PASS with 0 errors.

- [ ] **Step 5: Commit**

```bash
git add desk/src/lib/components/Sidebar.svelte
git commit -m "feat(desk): add workspace selector and workspace-filtered sidebar items"
```

---

### Task 4: Internal Desk Navigation Links & SDK Updates

**Files:**
- Modify: `desk/src/lib/components/Workspace.svelte`
- Modify: `desk/src/lib/components/ListView.svelte`
- Modify: `desk/src/lib/components/FormView.svelte`
- Modify: `desk/src/lib/controls/LinkControl.svelte`
- Modify: `desk/src/lib/desk-sdk.ts`

- [ ] **Step 1: Update `Workspace.svelte` links**

In `desk/src/lib/components/Workspace.svelte`:
Update `href` helper so that shortcuts, links, and cards prefix with `ws.name`:
```ts
const href = (s: any) => s.route || (s.doctype ? `/app/${encodeURIComponent(ws?.name || "")}/${encodeURIComponent(s.doctype)}` : s.report ? `/app/${encodeURIComponent(ws?.name || "")}/report/${encodeURIComponent(s.report)}` : "#");
```

- [ ] **Step 2: Update `ListView.svelte` row and new button links**

In `desk/src/lib/components/ListView.svelte`:
Read `workspace` from route params or fallback:
`const workspace = $derived(page.params.workspace || "");`
Prefix the row click and "New" button href:
- Row click: `/app/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}/${encodeURIComponent(r.name)}`
- New button: `/app/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}/new`

- [ ] **Step 3: Update `FormView.svelte` redirects and breadcrumbs**

In `desk/src/lib/components/FormView.svelte`:
`const workspace = $derived(page.params.workspace || "");`
Prefix redirects on delete, rename, save, and breadcrumb link:
- Breadcrumb: `/app/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}`
- New doc history replace: `/app/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}/new`
- After save / rename: `/app/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}/${encodeURIComponent(saved.name)}`

- [ ] **Step 4: Update `LinkControl.svelte` open button**

In `desk/src/lib/controls/LinkControl.svelte`:
`const workspace = $derived(page.params.workspace || "");`
Link button href:
`/app/${encodeURIComponent(workspace)}/${encodeURIComponent(target)}/${encodeURIComponent(value)}`

- [ ] **Step 5: Run tests and type check**

Run: `npm --prefix desk test && npm --prefix desk run check`  
Expected: PASS with 0 errors.

- [ ] **Step 6: Commit**

```bash
git add desk/src/lib/components/Workspace.svelte desk/src/lib/components/ListView.svelte desk/src/lib/components/FormView.svelte desk/src/lib/controls/LinkControl.svelte desk/src/lib/desk-sdk.ts
git commit -m "feat(desk): use workspace prefix in internal navigation links and views"
```

---

### Task 5: End-to-End Verification & Desk Build

**Files:**
- None (verification only)

- [ ] **Step 1: Run Desk test suite**

Run: `npm --prefix desk test`  
Expected: 100% tests pass.

- [ ] **Step 2: Run Svelte type checks**

Run: `npm --prefix desk run check`  
Expected: 0 errors, 0 warnings.

- [ ] **Step 3: Build the Desk**

Run: `npm --prefix desk run build`  
Expected: SvelteKit builds successfully into `desk/build` with no routing conflicts.

- [ ] **Step 4: Run repository checks**

Run: `make test-go`  
Expected: All Go unit and acceptance tests pass.

- [ ] **Step 5: Commit any final adjustments**

```bash
git commit -m "chore(desk): build and verify workspace selector"
```
