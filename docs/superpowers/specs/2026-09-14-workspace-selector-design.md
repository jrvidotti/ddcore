# Workspace Selector & URL Prefix Routing Design Specification

**Date:** 2026-09-14  
**Status:** Approved  
**Topic:** Desk Workspace Selector & Route Prefixing  

## 1. Overview & Objectives

In `ddcore`, multiple applications and workspaces can be configured simultaneously (e.g., `alugueis` and `manutencao`). Previously, the desk sidebar rendered every workspace concatenated sequentially in a single vertical navigation list.

This design introduces:
1. A **Workspace Selector** dropdown in the Desk sidebar, allowing users to switch between available workspaces cleanly.
2. An **isolated workspace navigation tree**, so only the active workspace's sidebar items are rendered.
3. **Workspace prefix routing** (`/app/[workspace]/...`), prefixing DocType, document, report, and dashboard URLs with the active workspace identifier, while maintaining backward-compatible automatic redirects for legacy and short routes (`/app/[doctype]`, `/app/workspace/[name]`, etc.).

---

## 2. Architecture & Route Structure

### 2.1 Route Hierarchy

Under SvelteKit (`desk/src/routes/app`):

- **`/app`**  
  Root entry point. Redirects to `/app/[workspace]` where `[workspace]` is the last active workspace remembered in `localStorage`, or the first allowed workspace in `boot.data.workspaces`.

- **`/app/[workspace]`**  
  Workspace dashboard view (renders `Workspace.svelte` for the given workspace `name`). Replaces `/app/workspace/[name]`.

- **`/app/[workspace]/[doctype]`**  
  DocType list view (renders `ListView.svelte`).

- **`/app/[workspace]/[doctype]/new`**  
  New document form view (renders `FormView.svelte` for a new record).

- **`/app/[workspace]/[doctype]/[name]`**  
  Existing document form view (renders `FormView.svelte` for record `[name]`).

- **`/app/[workspace]/report/[name]`**  
  Report view (renders `ReportView.svelte` for report `[name]`).

- **Global User Routes (un-prefixed):**
  - `/app/notifications`: Notification inbox across all workspaces.
  - `/app/todo`: Assigned tasks across all workspaces.
  - `/app/profile`: User profile settings.

### 2.2 SvelteKit Dynamic Route Structure

To support `/app/[workspace]/...` while handling legacy routes without route conflicts in SvelteKit:

Since SvelteKit does not allow sibling parameter folders like `[workspace]` and `[doctype]` at the same root `/app/`, the directory structure becomes:

```text
desk/src/routes/app/
├── +page.svelte                      # Redirects to /app/[workspace]
├── notifications/
│   └── +page.svelte                  # Global notifications
├── todo/
│   └── +page.svelte                  # Global todo
├── profile/
│   └── +page.svelte                  # Global profile
├── workspace/                        # Backward-compatibility redirect
│   └── [name]/
│       └── +page.svelte              # Redirects /app/workspace/[name] -> /app/[name]
└── [workspace]/
    ├── +page.svelte                  # Workspace dashboard OR redirect if [workspace] is a doctype
    ├── [doctype]/
    │   ├── +page.svelte              # ListView OR redirect if [workspace] was a doctype
    │   ├── new/
    │   │   └── +page.svelte          # FormView (new)
    │   └── [name]/
    │       └── +page.svelte          # FormView (existing)
    └── report/
        └── [name]/
            └── +page.svelte          # ReportView
```

### 2.3 Backward-Compatibility & Route Resolution

When a request arrives at `/app/[segment]`:
- If `[segment]` matches a known workspace `name` (case-insensitive comparison with `boot.data.workspaces`), it renders the workspace dashboard.
- If `[segment]` does not match a workspace, but matches a known `DocType`:
  It resolves the owning workspace for that DocType and redirects to `/app/[resolvedWorkspace]/[segment]`.
- If a request arrives at `/app/[segment1]/[segment2]`:
  - If `[segment1]` is a DocType rather than a workspace (e.g. `/app/Contrato/CTR-0001` or `/app/Contrato/new`):
    It resolves the owning workspace for `[segment1]` and redirects to `/app/[resolvedWorkspace]/[segment1]/[segment2]`.

---

## 3. State Management & Resolution Logic

A dedicated helper module `desk/src/lib/components/sidebar-workspace.ts` provides pure, testable functions:

### 3.1 Data Structures
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
```

### 3.2 Resolution Functions
1. `resolveActiveWorkspace(params: { currentPath: string; workspaces: WorkspaceItem[]; doctypes?: Record<string, { app?: string }>; remembered?: string }): WorkspaceItem | null`
   - Checks if `currentPath` starts with `/app/[workspace]` and matches any workspace `name`.
   - If not directly on a workspace path, checks if current route contains a doctype or report declared in a workspace's `sidebar`.
   - If no match in sidebar, checks if the doctype's `app` matches any workspace's `app`.
   - If path is global (`/app/notifications`, `/app/todo`, `/app/profile`), falls back to `remembered` workspace.
   - Defaults to `workspaces[0]` or `null`.

2. `resolveWorkspaceForDoctype(doctype: string, workspaces: WorkspaceItem[], doctypesMeta?: Record<string, { app?: string }>): string | null`
   - Finds which workspace owns the given DocType.

3. `rememberWorkspace(name: string): void` and `getRememberedWorkspace(): string`
   - Safely saves and retrieves `ddcore_workspace` from `localStorage`.

4. `workspaceItemHref(workspaceName: string, item: { route?: string; doctype?: string; report?: string }): string`
   - Generates the workspace-prefixed URL:
     - `item.route` if already an explicit absolute route.
     - `/app/${workspaceName}/${doctype}` for doctypes.
     - `/app/${workspaceName}/report/${report}` for reports.

---

## 4. Desk Sidebar Component (`Sidebar.svelte`)

### 4.1 Header & Workspace Switcher
Positioned beneath the site brand (`.brand`) and above `<nav>`:
- **Multiple Workspaces (`workspaces.length > 1`):**
  - Renders `.workspace-switcher.dropdown`.
  - Button shows the active workspace icon, label, and a chevron toggle.
  - Dropdown menu lists all accessible workspaces with their icons and an active indicator.
  - Clicking a workspace navigates to `/app/[workspace]` and closes the menu.
- **Single Workspace (`workspaces.length === 1`):**
  - Renders a clean, non-interactive heading with workspace icon and label.
- **No Workspaces:**
  - Omits the workspace switcher block.

### 4.2 Navigation List (`<nav>`)
- Top global items: `Notifications` and `To-Do`.
- Workspace section: Renders **only** the active workspace's sidebar groups and items.
- Bottom administration: Collapsible `System` section for users with `System Manager` role (listing core DocTypes: `User`, `Role`, etc.).
- Footer (`.foot`): Current user profile dropdown.

### 4.3 Interaction & Accessibility
- Clicking outside `.dropdown` closes the workspace menu.
- Pressing `Escape` closes the workspace menu.
- Proper ARIA attributes (`aria-haspopup="menu"`, `aria-expanded`).

---

## 5. Verification & Testing Strategy

1. **Unit Tests (`sidebar-workspace.test.ts`):**
   - Workspace resolution from direct route `/app/Alugueis`.
   - Workspace resolution from prefixed route `/app/Alugueis/Contrato`.
   - Inferred resolution for legacy un-prefixed doctype `/app/Contrato`.
   - Inferred resolution for legacy report `/app/report/Recebimentos`.
   - Persistence and retrieval in `localStorage`.
   - Single workspace and empty workspace edge cases.

2. **Integration & Build Tests:**
   - Run `make test` to ensure all Go acceptance tests and Desk TypeScript/Svelte checks pass.
   - Verify Desk builds without routing conflicts (`vite build` in `desk`).
