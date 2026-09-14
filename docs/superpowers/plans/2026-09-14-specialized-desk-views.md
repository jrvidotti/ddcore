# Specialized Desk Views (Calendar, Cards, View Switcher) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement specialized Desk views for DocType listings: responsive Card view (automatic default on mobile viewports < 768px), Calendar view for date-centric records with month navigation and click-to-create, a segmented view switcher, URL and localStorage state synchronization, and declarative SDK configuration in `defineListView`.

**Architecture:** Pure helper functions in `desk/src/lib/components/list-state.ts` resolve active view modes and serialize `?view=` parameters with 100% test coverage. `ListView.svelte` acts as orchestrator, managing data queries (including month range queries for Calendar mode) and URL/storage sync. Display is decomposed into modular presenters: `TableView.svelte`, `CardView.svelte`, and `CalendarView.svelte`. SDK definitions in `packages/desk-sdk/src/index.ts` and `desk/src/lib/desk-sdk.ts` expose typed configuration.

**Tech Stack:** Svelte 5 (runes: `$state`, `$derived`, `$props`), SvelteKit 2, TypeScript, Vitest.

**Spec:** [`docs/superpowers/specs/2026-09-14-specialized-desk-views-design.md`](file:///Users/junior/dev/ddcore/docs/superpowers/specs/2026-09-14-specialized-desk-views-design.md)

## Global Constraints

- Svelte 5 runes (`$state`, `$derived`, `$props`, `$effect`) must be used for Desk reactivity.
- User-facing text must use English as canonical string and `__("...")` for translation.
- Never edit `.ddcore/types.d.ts` by hand; use `./bin/ddcore types`.
- Verify translations with `./bin/ddcore i18n extract --all --lang pt-BR --check`.
- All tests must pass: `npm --prefix desk test` and `make check`.

---

### Task 1: Desk SDK Type Definitions

**Files:**
- Modify: `packages/desk-sdk/src/index.ts`
- Modify: `desk/src/lib/desk-sdk.ts`

**Interfaces:**
- Produces:
  - `export type DeskViewMode = "list" | "calendar" | "cards";`
  - `export interface CalendarViewOptions<T extends BaseDoc = BaseDoc>`
  - `export interface CardViewOptions<T extends BaseDoc = BaseDoc>`
  - Extended `ListViewOptions<T>` with `views?: DeskViewMode[]`, `calendar?: CalendarViewOptions<T>`, `card?: CardViewOptions<T>`

- [ ] **Step 1: Update `packages/desk-sdk/src/index.ts`**

Add `DeskViewMode`, `CalendarViewOptions`, `CardViewOptions`, and extend `ListViewOptions`:

```ts
export type DeskViewMode = "list" | "calendar" | "cards";

export interface CalendarViewOptions<T extends BaseDoc = BaseDoc> {
  /** Required: Date or Datetime field to plot records on the calendar */
  field: keyof T & string;
  /** Optional: Datetime or Date field for range spans */
  endField?: keyof T & string;
  /** Field shown as label inside the calendar chip (defaults to titleField or name) */
  titleField?: keyof T & string;
  /** Field determining chip color (e.g. "status", uses optionColors automatically) */
  colorField?: keyof T & string;
}

export interface CardViewOptions<T extends BaseDoc = BaseDoc> {
  title?: keyof T & string;
  subtitle?: keyof T & string;
  dateField?: keyof T & string;
  indicator?: (row: T) => { label: string; color: string } | null | undefined;
  badges?: (row: T) => { label: string; color: string }[] | null | undefined;
}
```

And in `ListViewOptions<T>`:
```ts
  /** Allowed views for this DocType; defaults to ["list", "cards"] (or ["list", "calendar", "cards"] when calendar is defined) */
  views?: DeskViewMode[];
  calendar?: CalendarViewOptions<T>;
  card?: CardViewOptions<T>;
```

- [ ] **Step 2: Update `desk/src/lib/desk-sdk.ts`**

Import and expose `DeskViewMode`, `CalendarViewOptions`, `CardViewOptions` to ensure compile-time parity between runtime SDK and packages.

- [ ] **Step 3: Run check to verify types compile**

Run: `npm --prefix desk run check`
Expected: PASS with 0 errors.

- [ ] **Step 4: Commit**

```bash
git add packages/desk-sdk/src/index.ts desk/src/lib/desk-sdk.ts
git commit -m "feat(desk-sdk): add calendar and card view options to defineListView"
```

---

### Task 2: View State Resolution & URL Parameter Handling

**Files:**
- Modify: `desk/src/lib/components/list-state.ts`
- Test: `desk/src/lib/components/list-state.test.ts`

**Interfaces:**
- Produces:
  - `export function resolveAllowedViews(settings?: { views?: string[]; calendar?: { field?: string } }): string[]`
  - `export function resolveActiveView(allowedViews: string[], urlView?: string | null, storedView?: string | null, isMobile?: boolean): string`
  - Extended `ListUrlState` with `view?: string`
  - Updated `listStateFromSearchParams` and `listStateToSearchParams`

- [ ] **Step 1: Write failing tests in `desk/src/lib/components/list-state.test.ts`**

```ts
import { resolveAllowedViews, resolveActiveView } from "./list-state";

describe("view resolution", () => {
  it("defaults allowed views to list and cards when no calendar configured", () => {
    expect(resolveAllowedViews({})).toEqual(["list", "cards"]);
  });

  it("includes calendar in allowed views when calendar field configured", () => {
    expect(resolveAllowedViews({ calendar: { field: "due_date" } })).toEqual(["list", "calendar", "cards"]);
  });

  it("respects explicit views array from settings", () => {
    expect(resolveAllowedViews({ views: ["calendar", "list"] })).toEqual(["calendar", "list"]);
  });

  it("resolves active view giving highest priority to URL param", () => {
    expect(resolveActiveView(["list", "cards", "calendar"], "calendar", "cards", false)).toBe("calendar");
  });

  it("resolves active view falling back to stored view if URL param is absent or invalid", () => {
    expect(resolveActiveView(["list", "cards"], "unknown", "cards", false)).toBe("cards");
    expect(resolveActiveView(["list", "cards"], null, "cards", false)).toBe("cards");
  });

  it("defaults to cards on mobile (< 768px) when no URL or stored view exists", () => {
    expect(resolveActiveView(["list", "cards"], null, null, true)).toBe("cards");
  });

  it("defaults to primary view (first item) on desktop when no URL or stored view exists", () => {
    expect(resolveActiveView(["calendar", "list", "cards"], null, null, false)).toBe("calendar");
    expect(resolveActiveView(["list", "cards"], null, null, false)).toBe("list");
  });
});
```

- [ ] **Step 2: Run test to verify failure**

Run: `npm --prefix desk test src/lib/components/list-state.test.ts`
Expected: FAIL with missing exports `resolveAllowedViews` / `resolveActiveView`.

- [ ] **Step 3: Implement functions in `desk/src/lib/components/list-state.ts`**

```ts
export function resolveAllowedViews(settings?: { views?: string[]; calendar?: { field?: string } }): string[] {
  if (settings?.views && settings.views.length > 0) {
    return [...settings.views];
  }
  if (settings?.calendar?.field) {
    return ["list", "calendar", "cards"];
  }
  return ["list", "cards"];
}

export function resolveActiveView(
  allowedViews: string[],
  urlView?: string | null,
  storedView?: string | null,
  isMobile?: boolean,
): string {
  if (urlView && allowedViews.includes(urlView)) {
    return urlView;
  }
  if (storedView && allowedViews.includes(storedView)) {
    return storedView;
  }
  if (isMobile && allowedViews.includes("cards")) {
    return "cards";
  }
  return allowedViews[0] || "list";
}
```

Update `ListUrlState` to include `view?: string`. In `listStateFromSearchParams`, read `params.get("view")`. In `listStateToSearchParams`, if `state.view` is set and not "list", set `params.set("view", state.view)`.

- [ ] **Step 4: Run tests to verify they pass**

Run: `npm --prefix desk test src/lib/components/list-state.test.ts`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add desk/src/lib/components/list-state.ts desk/src/lib/components/list-state.test.ts
git commit -m "feat(desk): add view mode resolution and URL state persistence"
```

---

### Task 3: Icon & Presenter Components (`TableView`, `CardView`, `CalendarView`)

**Files:**
- Modify: `desk/src/lib/components/Icon.svelte`
- Create: `desk/src/lib/components/views/TableView.svelte`
- Create: `desk/src/lib/components/views/CardView.svelte`
- Create: `desk/src/lib/components/views/CalendarView.svelte`

- [ ] **Step 1: Add `layout-grid` icon in `Icon.svelte`**

Add `"layout-grid"` path:
```ts
"layout-grid": "M3 3h7v7H3z M14 3h7v7h-7z M14 14h7v7h-7z M3 14h7v7H3z",
```

- [ ] **Step 2: Create `desk/src/lib/components/views/TableView.svelte`**

Move standard tabular markup cleanly from `ListView.svelte` into `TableView.svelte`.
Handles:
- Checkbox header and row selection
- Column sorting indicators
- Link and Dynamic Link resolution
- Status indicator and extra badges
- Formatted values and modified time ago

- [ ] **Step 3: Create `desk/src/lib/components/views/CardView.svelte`**

Implement touch-friendly card grid:
- `display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px;`
- Header: Selection checkbox (stops propagation), title (`card.title` or `meta.doctype.titleField` or `name`), status indicator (`card.indicator` or standard status pill)
- Subtitle: `card.subtitle`
- Key fields: First 2–3 `inListView` fields formatted cleanly
- Footer: Date (`card.dateField`) and badges (`card.badges` or `settings.badges`)
- Click card navigates to document form

- [ ] **Step 4: Create `desk/src/lib/components/views/CalendarView.svelte`**

Implement monthly calendar grid:
- Navigation bar: Previous (`<`), Today, Next (`>`), and month/year title using site timezone
- 7 weekday columns using `dayNames()` from `date-format.ts`
- 35 or 42 day cells from `getCalendarDays(viewYear, viewMonth)`
- Plots records by `row[calendar.field]` (normalized to `YYYY-MM-DD`)
- Event chip displays `row[calendar.titleField || meta.doctype.titleField || "name"]` with background/text from `statusColor(row[calendar.colorField || "status"])`
- Click event chip navigates to document form
- Click day cell triggers "+ New" navigation pre-filling `calendar.field` date: `${wsPrefix}/${doctype}/new?${calendar.field}=${day.iso}`
- Navigating months notifies parent via `onMonthChange(year, month, gridStartIso, gridEndIso)`

- [ ] **Step 5: Run desk checks**

Run: `npm --prefix desk run check`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add desk/src/lib/components/Icon.svelte desk/src/lib/components/views/TableView.svelte desk/src/lib/components/views/CardView.svelte desk/src/lib/components/views/CalendarView.svelte
git commit -m "feat(desk): create TableView, CardView, and CalendarView presenter components"
```

---

### Task 4: `ListView.svelte` Orchestration & Integration

**Files:**
- Modify: `desk/src/lib/components/ListView.svelte`

- [ ] **Step 1: Integrate View Resolution & Switcher**

- Calculate `allowedViews = $derived(resolveAllowedViews(settings))`.
- Compute `isMobile` via window width detection (`window.innerWidth < 768`).
- Resolve `currentView` using `resolveActiveView(allowedViews, page.url.searchParams.get("view"), localStorage.getItem("ddcore_view_" + doctype), isMobile)`.
- In `page-head` toolbar, render segmented switcher:
  ```svelte
  {#if allowedViews.length > 1}
    <div class="view-switcher">
      {#if allowedViews.includes("list")}
        <button class="btn icon" class:active={currentView === "list"} onclick={() => setView("list")} title={__("List")} aria-label={__("List")}><Icon name="list" size={14} /></button>
      {/if}
      {#if allowedViews.includes("calendar")}
        <button class="btn icon" class:active={currentView === "calendar"} onclick={() => setView("calendar")} title={__("Calendar")} aria-label={__("Calendar")}><Icon name="calendar" size={14} /></button>
      {/if}
      {#if allowedViews.includes("cards")}
        <button class="btn icon" class:active={currentView === "cards"} onclick={() => setView("cards")} title={__("Cards")} aria-label={__("Cards")}><Icon name="layout-grid" size={14} /></button>
      {/if}
    </div>
  {/if}
  ```

- [ ] **Step 2: Implement `setView(mode)` and Calendar Date Querying**

- `setView(mode)`:
  - Updates `currentView = mode`
  - Persists to `localStorage.setItem("ddcore_view_" + doctype, mode)`
  - Updates URL with `?view=${mode}` preserving filters/search
  - Calls `load()`
- In `load()`:
  - If `currentView === "calendar"` and `settings.calendar?.field`:
    - Add calendar date range filters: `[calendar.field, '>=', gridStartIso]` and `[calendar.field, '<=', gridEndIso]`
    - Query with limit 500
  - Else:
    - Standard pagination (`limit: pageSize`, `start`)

- [ ] **Step 3: Render Active Presenter**

```svelte
{#if currentView === "list"}
  <TableView ... />
{:else if currentView === "cards"}
  <CardView ... />
{:else if currentView === "calendar"}
  <CalendarView ... />
{/if}
```

- [ ] **Step 4: Run tests and type check**

Run: `npm --prefix desk test && npm --prefix desk run check`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add desk/src/lib/components/ListView.svelte
git commit -m "feat(desk): integrate specialized views and switcher into ListView"
```

---

### Task 5: App Fixture Configuration, Translations & Full Verification

**Files:**
- Modify: `apps/testapp/doctypes/task/task.form.ts`
- Modify: `core/translations/pt-BR.csv`

- [ ] **Step 1: Configure `defineListView` on `Task` in `apps/testapp`**

Add declarative configuration to `apps/testapp/doctypes/task/task.form.ts`:
```ts
import { defineForm, defineListView, ddcore } from "@ddcore/desk-sdk";

defineListView("Task", {
  views: ["list", "calendar", "cards"],
  calendar: {
    field: "due_date",
    endField: "completed_at",
    titleField: "title",
    colorField: "status",
  },
  card: {
    title: "title",
    subtitle: "project",
    dateField: "due_date",
  },
});
```

- [ ] **Step 2: Extract & update translations**

Run: `./bin/ddcore i18n extract --all --lang pt-BR`
Ensure keys `List`, `Calendar`, `Cards`, `Today`, `Previous month`, `Next month` have valid Portuguese translations in `core/translations/pt-BR.csv`.

- [ ] **Step 3: Run `make check`**

Run: `make check`
Expected: PASS (svelte-check, types, core tsconfig, testapp tsconfig, i18n check all clean).

- [ ] **Step 4: Run `make test`**

Run: `make test`
Expected: PASS (Go tests + desk Vitest + testapp tests).

- [ ] **Step 5: Commit**

```bash
git add apps/testapp/doctypes/task/task.form.ts core/translations/pt-BR.csv
git commit -m "feat: configure Task list view in testapp and update translations"
```
