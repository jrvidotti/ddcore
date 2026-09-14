# Specialized Desk Views: Calendar and Responsive Card View Design Specification

**Date:** 2026-09-14  
**Status:** Approved  
**Topic:** Specialized Desk Views (Calendar, Responsive Card View, View Switcher) — OPS-08  

---

## 1. Overview & Objectives

In `ddcore`, `/app/[workspace]/[doctype]` previously only rendered a single tabular view (`<ListView />`). This had notable limitations:
1. **No view switcher:** An app could not toggle between List, Calendar, and Card views from the toolbar.
2. **Missing Calendar view for date-centric records:** Although `date-format.ts` provides `getCalendarDays()` and monthly grid helpers, there was no Desk calendar view mapping documents to calendar cells based on date/datetime fields (such as `due_date` or `start_date`/`end_date`).
3. **Suboptimal mobile UX on wide tables:** On mobile viewports (< 768px), tabular grids cause horizontal scrolling, truncated headers, and poor touch ergonomics.

This specification addresses **OPS-08** by delivering:
- A responsive **Card view** that serves as the automatic default on mobile viewports (< 768px).
- A **Calendar view** mapping documents to a monthly grid with timezone awareness, month navigation, status-colored chips, and click-to-create pre-filling.
- A **segmented view switcher** in the `ListView` toolbar: `[ ☰ List | 📅 Calendar | 🗂 Cards ]`.
- **View state persistence** prioritizing: URL query parameter (`?view=...`), then user local preference (`localStorage`), then responsive default (Cards on mobile, List/primary on desktop).
- Declarative configuration in `@ddcore/desk-sdk` via `defineListView`.

---

## 2. Desk SDK & Metadata Configuration

In `packages/desk-sdk/src/index.ts` and `desk/src/lib/desk-sdk.ts`:

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

export interface ListViewOptions<T extends BaseDoc = BaseDoc> {
  /** Allowed views for this DocType; defaults to ["list", "cards"] (or ["list", "calendar", "cards"] when calendar is defined) */
  views?: DeskViewMode[];
  /** Calendar view configuration */
  calendar?: CalendarViewOptions<T>;
  /** Card view customization */
  card?: CardViewOptions<T>;
  // ... existing options: columns, filters, orderBy, pageSize, formatters, indicator, badges, etc.
}
```

---

## 3. View Resolution & State Persistence

### 3.1 Allowed Views Resolution
1. If `settings.views` is explicitly provided, use it directly (e.g. `["list", "calendar", "cards"]`).
2. If `settings.views` is not provided:
   - If `settings.calendar?.field` is configured, allowed views default to `["list", "calendar", "cards"]`.
   - Otherwise, allowed views default to `["list", "cards"]`.

### 3.2 Active View Determination
The active view mode is determined in order of precedence:
1. **URL parameter:** If `?view=<mode>` is present in the query string and valid in `allowedViews`, it wins.
2. **Local preference:** If `localStorage.getItem(`ddcore_view_${doctype}`)` is present and valid in `allowedViews`, it is applied next.
3. **Responsive default:**
   - If viewport width is < 768px (mobile): defaults to `"cards"` (if `"cards"` is allowed).
   - If desktop (>= 768px): defaults to the primary view (first item in `allowedViews`, typically `"list"`).

### 3.3 Switching Views & URL Synchronization
- When the user selects a view in the segmented control:
  - The choice is persisted to `localStorage.setItem(`ddcore_view_${doctype}`, mode)`.
  - The URL search param is updated to `?view=<mode>` (preserving all other filters, search, and ordering) via SvelteKit `goto`.
  - If switched to `"calendar"`, the calendar data loader is triggered.

---

## 4. Querying & Data Flow

### 4.1 List and Card Modes
- Retain standard pagination (`limit: pageSize`, `start`, `page`, `page_size`), filters, search, and ordering.
- Bulk selections are shared between List and Card modes.

### 4.2 Calendar Mode
- The Calendar view maintains an active year and month (initialized using the site's `today()` date from `$lib/datetime`).
- To prevent truncation of monthly events by small list page sizes (e.g. 20 records), calendar mode queries:
  - Date filter: `[calendar.field, '>=', gridStartIso]` and `[calendar.field, '<=', gridEndIso]` (covering previous and next month overflow days in the visible 35- or 42-day grid).
  - Limit: up to 500 records.
  - Standard filters and search queries are preserved and unified.
- Navigating previous/next month updates the active month and re-fetches records for the new date window.

---

## 5. UI Architecture & Presenter Components

### 5.1 Orchestrator: `ListView.svelte`
- Retains top-level page header, bulk actions, search, standard filters, export modal, and state synchronization.
- Segmented view switcher button group:
  ```svelte
  {#if allowedViews.length > 1}
    <div class="btn-group view-switcher">
      {#if allowedViews.includes("list")}
        <button class="btn icon" class:active={currentView === "list"} onclick={() => setView("list")} title={__("List")}>
          <Icon name="list" size={14} />
        </button>
      {/if}
      {#if allowedViews.includes("calendar")}
        <button class="btn icon" class:active={currentView === "calendar"} onclick={() => setView("calendar")} title={__("Calendar")}>
          <Icon name="calendar" size={14} />
        </button>
      {/if}
      {#if allowedViews.includes("cards")}
        <button class="btn icon" class:active={currentView === "cards"} onclick={() => setView("cards")} title={__("Cards")}>
          <Icon name="layout-grid" size={14} />
        </button>
      {/if}
    </div>
  {/if}
  ```

### 5.2 Presenters

#### `TableView.svelte` (`desk/src/lib/components/views/TableView.svelte`)
- Tabular representation extracted from the original `ListView.svelte` table.
- Supports column sorting, row select checkboxes, link title resolution, status pills, and modified timestamp.

#### `CardView.svelte` (`desk/src/lib/components/views/CardView.svelte`)
- Touch-friendly responsive card grid (`display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px;`).
- Each card contains:
  - **Header:** Selection checkbox, document title (`settings.card?.title` or `meta.doctype.titleField` or `name`), and status pill (`settings.card?.indicator` or `statusColor`).
  - **Subtitle:** `settings.card?.subtitle` (if declared).
  - **Body:** First 2–3 `inListView` fields formatted cleanly with resolved Link titles.
  - **Footer:** Date (`settings.card?.dateField` or first date field) and badges (`settings.card?.badges` or `settings.badges`).
- Clicking the card navigates to `/app/[workspace]/[doctype]/[name]`.
- Checkbox click stops propagation to avoid triggering navigation.

#### `CalendarView.svelte` (`desk/src/lib/components/views/CalendarView.svelte`)
- **Header:** Month navigation `[ < ] [ Today ] [ > ]` with month and year title (`monthTitles()[viewMonth - 1] viewYear`).
- **Weekday Headers:** 7 columns using `dayNames()` from `date-format.ts` (Sunday first).
- **Day Cells:** 35 or 42 cells generated by `getCalendarDays(viewYear, viewMonth)`.
  - Cells outside current month are dimmed.
  - Current day (`today()`) is accented.
  - Clicking an empty day cell or "+ New" navigates to `${wsPrefix}/${doctype}/new?${calendar.field}=${day.iso}`, automatically pre-filling the date in the form.
- **Event Chips:**
  - Plotted on their corresponding day (matching `row[calendar.field]`).
  - Labeled with `row[calendar.titleField || meta.doctype.titleField || "name"]`.
  - Background and pill color derived from `statusColor(row[calendar.colorField || "status"])`.
  - Clicking an event chip navigates to `${wsPrefix}/${doctype}/${row.name}`.

---

## 6. Internationalisation (i18n)

All user-facing strings are English keys translated in `core/translations/pt-BR.csv`:
- `List` -> `Lista`
- `Calendar` -> `Calendário`
- `Cards` -> `Cartões`
- `Today` -> `Hoje`
- `Previous month` -> `Mês anterior`
- `Next month` -> `Próximo mês`

Checked and verified with `./bin/ddcore i18n extract --all --lang pt-BR --check`.

---

## 7. Testing Strategy

1. **Unit Tests (`desk/src/lib/components/list-state.test.ts` / `desk/src/lib/components/views/*.test.ts`):**
   - Verification of view mode resolution from URL query params, localStorage, and viewport width.
   - Verification of serialization/deserialization of `?view=`.
   - Verification of calendar day grouping and card item field extraction.
2. **Type Checking:**
   - `npm run check` in `desk/` ensuring full TypeScript compliance.
   - Core and testapp type checks (`tsc -p ... --noEmit`).
3. **Automated Verification:**
   - `make test-desk` (vitest + svelte-check).
   - `make check` (complete desk check + i18n checks).
