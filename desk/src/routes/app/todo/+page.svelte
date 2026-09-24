<script lang="ts">
  // The To-Do list: the caller's assignments through /api/todo/pending, which
  // enforces who sees what (the assigner too, and never a task on a document
  // the caller can't read), drawn with the generic list's views.
  import { focusTrap } from "$lib/focus-trap";
  import { onDestroy, onMount, untrack } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { PendingWork, refreshPendingCount, TODO_WINDOW_LIMIT } from "$lib/assignments.svelte";
  import { api, type ToDoDoc } from "$lib/api";
  import { __, boot, doctypeLabel, siteName } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import { getMeta, type Meta } from "$lib/meta";
  import { formatDate, statusColor } from "$lib/format";
  import { today } from "$lib/datetime";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { getCalendarDays } from "$lib/controls/date-format";
  import Icon from "$lib/components/Icon.svelte";
  import LinkControl from "$lib/controls/LinkControl.svelte";
  import CalendarView from "$lib/components/views/CalendarView.svelte";
  import KanbanView from "$lib/components/views/KanbanView.svelte";
  import { workspaceFor } from "$lib/components/search-palette";
  import { getRememberedWorkspace, type WorkspaceItem } from "$lib/components/sidebar-workspace";
  import { isAssignmentOverdue, priorityBadgeClass } from "$lib/components/doc-sidebar-assignment";
  import {
    TODO_PAGE_SIZES, TODO_PRIORITIES, TODO_STATUSES, countTodoFilters, emptyTodoFilters, hasTodoFilters, nextTodoOrder,
    todoStateFromSearchParams, todoStateToSearchParams, type TodoDue, type TodoUrlState, type TodoView,
  } from "$lib/todo-list";

  const pw = new PendingWork();
  let meta = $state<Meta | null>(null);
  let view = $state<TodoView>("list");
  let search = $state("");
  let lastUrlSearch = "";
  let ready = false;
  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  let showNewModal = $state(false);
  // the filter bar starts hidden; a URL that carries filters opens it, so a
  // narrowed list never looks like the whole one
  let showFilters = $state(false);
  const activeFilters = $derived(countTodoFilters(pw.filters));

  // New ToDo form fields
  let newDescription = $state("");
  let newDate = $state("");
  let newPriority = $state<"Low" | "Medium" | "High" | "Urgent">("Medium");
  let newAllocatedTo = $state("");
  let savingNew = $state(false);

  const userField = {
    fieldname: "allocated_to",
    fieldtype: "Link",
    options: "User",
    label: "User",
  };

  const initialDate = today();
  let calendarYear = $state(Number(initialDate.slice(0, 4)));
  let calendarMonth = $state(Number(initialDate.slice(5, 7)));
  const initialDays = getCalendarDays(Number(initialDate.slice(0, 4)), Number(initialDate.slice(5, 7)));
  let gridStart = initialDays[0].iso;
  let gridEnd = initialDays[initialDays.length - 1].iso;

  const VIEW_KEY = "ddcore_view_todo";
  const calendar = { field: "date", titleField: "description", colorField: "priority" };
  const kanban = { field: "status", titleField: "description", subtitleField: "reference_id", colorField: "priority" };
  const dues = $derived<[TodoDue, string][]>([["overdue", __("Overdue")], ["today", __("Today")], ["week", __("Next 7 days")], ["none", __("No due date")]]);

  /** The workspace that owns a doctype, as a route prefix. */
  function wsPrefixFor(doctype: string) {
    const ws = workspaceFor(doctype, (boot.data?.workspaces || []) as WorkspaceItem[], boot.data?.doctypes, getRememberedWorkspace());
    return `/app/${encodeURIComponent(ws)}`;
  }
  const docHref = (doctype: string, id: string) => `${wsPrefixFor(doctype)}/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}`;
  // tasks open under this page, not under whatever workspace ToDo belongs to
  const TODO_BASE = "/app/todo";
  const todoHref = (id: string) => `${TODO_BASE}/${encodeURIComponent(id)}`;
  const todoPrefix = $derived(wsPrefixFor("ToDo"));
  const counterpartLabel = $derived(pw.scope === "assigned_by_me" ? __("Assigned To") : __("Assigned By"));
  const pageCount = $derived(Math.max(1, Math.ceil(pw.total / pw.limit)));
  const pageNumber = $derived(Math.floor(pw.offset / pw.limit) + 1);

  /** The grid's days, plus the undated tasks when today is on it: they are shown on today. */
  function calendarWindow(from: string, to: string) {
    const now = today();
    return from <= now && now <= to ? { date_from: from, date_to: to, undated: 1 } : { date_from: from, date_to: to };
  }

  /** A day picked on the calendar: the list of the tasks due that day. */
  function showDay(iso: string) {
    showFilters = true;
    update({ view: "list", filters: { ...pw.filters, due: "", date: iso } });
  }

  function currentState(): TodoUrlState {
    return {
      scope: pw.scope, status: pw.status, filters: { ...pw.filters }, orderBy: pw.orderBy,
      page: pageNumber, pageSize: pw.limit, view,
    };
  }

  function applyState(st: TodoUrlState) {
    pw.scope = st.scope;
    pw.status = st.status;
    pw.filters = st.filters;
    pw.orderBy = st.orderBy;
    pw.limit = st.pageSize;
    search = st.filters.q;
    view = st.view;
    pw.window = view === "calendar" ? calendarWindow(gridStart, gridEnd) : view === "kanban" ? { allStatuses: true } : null;
  }

  /** Puts the state in the URL (so it survives a reload and the back button) and loads it. */
  function commit(st: TodoUrlState) {
    applyState(st);
    const params = todoStateToSearchParams(st);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`/app/todo${lastUrlSearch}`, { noScroll: true, keepFocus: true, replaceState: false });
    void pw.load((st.page - 1) * st.pageSize);
  }

  const update = (patch: Partial<TodoUrlState>) => commit({ ...currentState(), page: 1, ...patch });
  const setFilter = (patch: Partial<TodoUrlState["filters"]>) => update({ filters: { ...pw.filters, ...patch } });

  function setView(next: TodoView) {
    try { window.localStorage.setItem(VIEW_KEY, next); } catch { /* the URL still carries the view */ }
    update({ view: next });
  }

  function onSearch() {
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => setFilter({ q: search }), 250);
  }

  function changeMonth(year: number, month: number, startIso: string, endIso: string) {
    calendarYear = year;
    calendarMonth = month;
    if (startIso === gridStart && endIso === gridEnd) return;
    gridStart = startIso;
    gridEnd = endIso;
    if (view === "calendar") {
      pw.window = calendarWindow(startIso, endIso);
      void pw.load(0);
    }
  }

  async function run(action: Promise<unknown>) {
    try { await action; } catch (e) { showError(e); }
  }
  const toggleDone = (row: ToDoDoc) => run(row.status === "Open" ? pw.complete(row.id) : pw.reopen(row.id));
  const moveCard = (id: string, value: string) => run(pw.setStatus(id, value as ToDoDoc["status"]));

  function sortMark(field: string) {
    const [f, dir] = pw.orderBy.split(" ");
    return f === field ? (dir === "asc" ? "↑" : "↓") : "";
  }
  function ariaSort(field: string) {
    const [f, dir] = pw.orderBy.split(" ");
    return f !== field ? "none" : dir === "asc" ? "ascending" : "descending";
  }
  const userTitle = (id?: string) => (id ? getLinkTitle("User", id) || id : "");

  $effect(() => {
    const current = page.url.search;
    untrack(() => {
      // only a back/forward or a link changes the URL under us; commit() keeps lastUrlSearch in step
      if (!ready || current === lastUrlSearch) return;
      lastUrlSearch = current;
      const st = todoStateFromSearchParams(page.url.searchParams);
      applyState(st);
      void pw.load((st.page - 1) * st.pageSize);
    });
  });

  onMount(() => {
    newAllocatedTo = boot.data?.user || "";
    const params = page.url.searchParams;
    const st = todoStateFromSearchParams(params);
    if (!params.has("view")) {
      let stored: string | null = null;
      try { stored = window.localStorage.getItem(VIEW_KEY); } catch { /* storage may be disabled */ }
      // a month grid and a board don't fit a phone; the list and the cards do
      if (stored === "cards" || (window.innerWidth >= 768 && (stored === "calendar" || stored === "kanban"))) st.view = stored;
    }
    lastUrlSearch = page.url.search;
    ready = true;
    showFilters = hasTodoFilters(st.filters);
    applyState(st);
    void pw.load((st.page - 1) * st.pageSize);
    getMeta("ToDo").then((m) => (meta = m)).catch(showError);
  });

  onDestroy(() => { clearTimeout(searchTimer); pw.destroy(); });

  async function createStandaloneTask() {
    if (!newDescription.trim()) return;
    savingNew = true;
    try {
      await api.insert("ToDo", {
        description: newDescription.trim(),
        date: newDate || undefined,
        priority: newPriority,
        allocated_to: newAllocatedTo || boot.data?.user,
        status: "Open",
      });
      showNewModal = false;
      newDescription = "";
      newDate = "";
      newPriority = "Medium";
      await pw.load(pw.offset);
      await refreshPendingCount();
    } catch (e) {
      showError(e);
    } finally {
      savingNew = false;
    }
  }
</script>

{#snippet pagination()}
  <div class="pagination">
    <span class="muted small" aria-live="polite">{__("{0} tasks", [pw.total])}</span>
    <span class="spacer"></span>
    <select class="input" style="width:auto" aria-label={__("Page size")} value={pw.limit} onchange={(e) => update({ pageSize: Number(e.currentTarget.value) })}>
      {#each TODO_PAGE_SIZES as n}<option value={n}>{n}</option>{/each}
    </select>
    <button class="btn sm" disabled={pw.loading || pageNumber <= 1} onclick={() => commit({ ...currentState(), page: pageNumber - 1 })} aria-label={__("Previous")}><Icon name="chevron-left" size={14} /></button>
    <span class="small muted">{pageNumber} / {pageCount}</span>
    <button class="btn sm" disabled={pw.loading || pageNumber >= pageCount} onclick={() => commit({ ...currentState(), page: pageNumber + 1 })} aria-label={__("Next")}><Icon name="chevron-right" size={14} /></button>
  </div>
{/snippet}

{#snippet taskActions(row: ToDoDoc)}
  <div class="task-actions">
    {#if row.status === "Open"}
      <button type="button" class="btn icon sm" title={__("Complete")} aria-label={__("Complete")} disabled={pw.pending === row.id} onclick={() => run(pw.complete(row.id))}><Icon name="check" size={14} /></button>
      <button type="button" class="btn icon sm" title={__("Revoke")} aria-label={__("Revoke")} disabled={pw.pending === row.id} onclick={() => run(pw.revoke(row.id))}><Icon name="x" size={14} /></button>
    {:else}
      <button type="button" class="btn icon sm" title={__("Reopen")} aria-label={__("Reopen")} disabled={pw.pending === row.id} onclick={() => run(pw.reopen(row.id))}><Icon name="rotate-ccw" size={14} /></button>
    {/if}
  </div>
{/snippet}

{#snippet doneToggle(row: ToDoDoc)}
  <input type="checkbox" checked={row.status === "Closed"} disabled={row.status === "Cancelled" || pw.pending === row.id}
    onchange={() => toggleDone(row)} aria-label={row.status === "Closed" ? __("Reopen") : __("Mark as completed")} />
{/snippet}

{#snippet reference(row: ToDoDoc)}
  <a class="ref-doc" href={docHref(row.reference_type!, row.reference_id!)}>
    <span class="muted">{doctypeLabel(row.reference_type!)}</span> · {getLinkTitle(row.reference_type!, row.reference_id!) || row.reference_id}
  </a>
{/snippet}

<svelte:head><title>{__("To-Do")} · {siteName()}</title></svelte:head>

<div class="page todo-page">
  <div class="page-head">
    <h1>{__("To-Do")}</h1>
    <div class="view-switcher">
      <button class="btn icon" class:active={view === "list"} aria-pressed={view === "list"} onclick={() => setView("list")} title={__("List")} aria-label={__("List")}><Icon name="list" size={14} /></button>
      <button class="btn icon" class:active={view === "calendar"} aria-pressed={view === "calendar"} onclick={() => setView("calendar")} title={__("Calendar")} aria-label={__("Calendar")}><Icon name="calendar" size={14} /></button>
      <button class="btn icon" class:active={view === "kanban"} aria-pressed={view === "kanban"} onclick={() => setView("kanban")} title={__("Kanban")} aria-label={__("Kanban")}><Icon name="square-kanban" size={14} /></button>
      <button class="btn icon" class:active={view === "cards"} aria-pressed={view === "cards"} onclick={() => setView("cards")} title={__("Cards")} aria-label={__("Cards")}><Icon name="layout-grid" size={14} /></button>
    </div>
    <button class="btn" onclick={() => pw.load(pw.offset)} disabled={pw.loading} title={__("Refresh")} aria-label={__("Refresh")}>
      <Icon name="refresh-cw" size={14} />
    </button>
    <button class="btn primary" onclick={() => (showNewModal = true)}>
      <Icon name="plus" size={14} />{__("New Task")}
    </button>
  </div>

  <div class="toolbar">
    <div class="scope-tabs" role="tablist">
      <button role="tab" class="tab-btn" class:active={pw.scope === "assigned_to_me"} aria-selected={pw.scope === "assigned_to_me"}
        onclick={() => update({ scope: "assigned_to_me", filters: { ...pw.filters, user: "" } })}>{__("Assigned to me")}</button>
      <button role="tab" class="tab-btn" class:active={pw.scope === "assigned_by_me"} aria-selected={pw.scope === "assigned_by_me"}
        onclick={() => update({ scope: "assigned_by_me", filters: { ...pw.filters, user: "" } })}>{__("Assigned by me")}</button>
    </div>
    <button class="btn" class:active={showFilters} aria-expanded={showFilters} aria-controls="todo-filters" onclick={() => (showFilters = !showFilters)}>
      <Icon name="filter" size={14} />{__("Filters")}{#if activeFilters}<span class="filter-count">{activeFilters}</span>{/if}
    </button>
  </div>

  {#if showFilters}
  <div class="card list-filters" id="todo-filters">
    <div class="filter-search">
      <label for="todo-search">{__("Search")}</label>
      <input id="todo-search" class="input" placeholder={__("Search…")} bind:value={search} oninput={onSearch} />
    </div>
    {#if view !== "kanban"}
    <div class="select-filter">
      <label for="todo-status">{__("Status")}</label>
      <select id="todo-status" class="input" value={pw.status} onchange={(e) => update({ status: e.currentTarget.value })}>
        {#each TODO_STATUSES as st}<option value={st}>{__(st)}</option>{/each}
        <option value="all">{__("All")}</option>
      </select>
    </div>
    {/if}
    <div class="select-filter">
      <label for="todo-priority">{__("Priority")}</label>
      <select id="todo-priority" class="input" value={pw.filters.priority} onchange={(e) => setFilter({ priority: e.currentTarget.value })}>
        <option value="">{__("All")}</option>
        {#each TODO_PRIORITIES as p}<option value={p}>{__(p)}</option>{/each}
      </select>
    </div>
    <div class="select-filter">
      <label for="todo-due">{__("Due Date")}</label>
      <select id="todo-due" class="input" value={pw.filters.date ? "date" : pw.filters.due} onchange={(e) => setFilter({ due: e.currentTarget.value as TodoDue, date: "" })}>
        {#if pw.filters.date}<option value="date">{formatDate(pw.filters.date)}</option>{/if}
        <option value="">{__("All")}</option>
        {#each dues as [value, label]}<option {value}>{label}</option>{/each}
      </select>
    </div>
    <div class="select-filter">
      <label for="todo-user">{counterpartLabel}</label>
      {#key pw.scope}
        <LinkControl id="todo-user" field={userField} value={pw.filters.user} onchange={(v) => setFilter({ user: v || "" })} />
      {/key}
    </div>
    <div class="filter-actions">
      <span class="label-spacer" aria-hidden="true">&nbsp;</span>
      <button class="btn" disabled={!hasTodoFilters(pw.filters)} onclick={() => update({ filters: emptyTodoFilters() })}><Icon name="x" size={14} />{__("Clear filters")}</button>
    </div>
  </div>
  {/if}

  {#if pw.error}
    <div class="card state" role="alert">
      <p>{pw.error}</p>
      <button class="btn" onclick={() => pw.load(pw.offset)}>{__("Retry")}</button>
    </div>
  {:else if view === "calendar"}
    {#if meta}
      {#if pw.total > pw.rows.length}<div class="view-notice muted small">{__("Showing the first {0} of {1} records; narrow the filters to see the rest", [pw.rows.length, pw.total])}</div>{/if}
      <CalendarView rows={pw.rows} {meta} doctype="ToDo" wsPrefix={todoPrefix} basePath={TODO_BASE} {calendar} viewYear={calendarYear} viewMonth={calendarMonth}
        onMonthChange={changeMonth} onDayClick={showDay} undatedOn={today()} />
    {/if}
  {:else if view === "kanban"}
    {#if meta}
      {#if pw.total > pw.rows.length}<div class="view-notice muted small">{__("Showing the first {0} of {1} records; narrow the filters to see the rest", [pw.rows.length, pw.total])}</div>{/if}
      <!-- cards move through the assignment endpoints, which check who may; Status itself is read-only on the form -->
      <KanbanView rows={pw.rows} {meta} doctype="ToDo" wsPrefix={todoPrefix} basePath={TODO_BASE} loading={pw.loading} {kanban} movable onMove={moveCard} />
    {/if}
  {:else if view === "cards"}
    <div class="todo-cards" aria-busy={pw.loading}>
      {#each pw.rows as row (row.id)}
        {@const counterpart = pw.scope === "assigned_by_me" ? row.allocated_to : row.assigned_by}
        <article class="card task-card" class:done={row.status !== "Open"}>
          <header>
            {@render doneToggle(row)}
            <a class="desc" href={todoHref(row.id)}>{row.description || row.id}</a>
            <span class="indicator {statusColor(row.status, meta?.doctype.fields.find((f) => f.fieldname === "status"))}">{__(row.status)}</span>
          </header>
          {#if row.reference_type && row.reference_id}<div class="ref">{@render reference(row)}</div>{/if}
          <dl>
            <div><dt>{__("Priority")}</dt><dd><span class="priority-badge {priorityBadgeClass(row.priority)}">{__(row.priority)}</span></dd></div>
            {#if row.date}<div><dt>{__("Due Date")}</dt><dd class:overdue={isAssignmentOverdue(row.date, row.status, initialDate)}>{formatDate(row.date)}</dd></div>{/if}
            {#if counterpart}<div><dt>{counterpartLabel}</dt><dd>{userTitle(counterpart)}</dd></div>{/if}
          </dl>
          <footer>{@render taskActions(row)}</footer>
        </article>
      {/each}
    </div>
    {#if !pw.loading && !pw.rows.length}
      <div class="card empty"><Icon name="check-square" size={24} /><p>{__("No tasks found")}</p></div>
    {/if}
    <div class="card cards-pagination">{@render pagination()}</div>
  {:else}
    <div class="card list-results" aria-busy={pw.loading}>
      <table class="grid">
        <thead>
          <tr>
            <th style="width:28px"><span class="sr-only">{__("Done")}</span></th>
            <th aria-sort={ariaSort("description")}><button class="sort" onclick={() => update({ orderBy: nextTodoOrder(pw.orderBy, "description") })}>{__("Description")} {sortMark("description")}</button></th>
            <th>{__("Reference")}</th>
            <th aria-sort={ariaSort("priority")}><button class="sort" onclick={() => update({ orderBy: nextTodoOrder(pw.orderBy, "priority") })}>{__("Priority")} {sortMark("priority")}</button></th>
            <th aria-sort={ariaSort("date")}><button class="sort" onclick={() => update({ orderBy: nextTodoOrder(pw.orderBy, "date") })}>{__("Due Date")} {sortMark("date")}</button></th>
            <th>{counterpartLabel}</th>
            <th aria-sort={ariaSort("status")}><button class="sort" onclick={() => update({ orderBy: nextTodoOrder(pw.orderBy, "status") })}>{__("Status")} {sortMark("status")}</button></th>
            <th class="num"><span class="sr-only">{__("Actions")}</span></th>
          </tr>
        </thead>
        <tbody>
          {#each pw.rows as row (row.id)}
            {@const counterpart = pw.scope === "assigned_by_me" ? row.allocated_to : row.assigned_by}
            <tr class="row" class:done={row.status !== "Open"}>
              <td>{@render doneToggle(row)}</td>
              <td class="desc"><a href={todoHref(row.id)}>{row.description || row.id}</a></td>
              <td class="ref">
                {#if row.reference_type && row.reference_id}{@render reference(row)}{/if}
              </td>
              <td class="nowrap"><span class="priority-badge {priorityBadgeClass(row.priority)}">{__(row.priority)}</span></td>
              <td class="nowrap" class:overdue={isAssignmentOverdue(row.date, row.status, initialDate)}>{formatDate(row.date)}</td>
              <td class="who">{userTitle(counterpart)}</td>
              <td class="nowrap"><span class="indicator {statusColor(row.status, meta?.doctype.fields.find((f) => f.fieldname === "status"))}">{__(row.status)}</span></td>
              <td class="num">{@render taskActions(row)}</td>
            </tr>
          {/each}
          {#if !pw.loading && !pw.rows.length}
            <tr><td colspan="8" class="empty"><Icon name="check-square" size={24} /><p>{__("No tasks found")}</p></td></tr>
          {/if}
        </tbody>
      </table>
      {@render pagination()}
    </div>
  {/if}
</div>

{#if showNewModal}
  <div
    class="modal-bg"
    use:focusTrap
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && (showNewModal = false)}
    onkeydown={(e) => e.key === "Escape" && (showNewModal = false)}
  >
    <div class="modal sm">
      <div class="head">
        <h3 style="display:flex;align-items:center;gap:8px">
          <Icon name="check-square" size={18} />
          {__("New Task")}
        </h3>
        <button class="btn icon" onclick={() => (showNewModal = false)} aria-label={__("Close")}>
          <Icon name="x" size={16} />
        </button>
      </div>
      <div class="body">
        <div class="form-group">
          <label for="new-task-desc" class="label reqd">{__("Description")}</label>
          <textarea
            id="new-task-desc"
            class="input"
            rows="3"
            placeholder={__("What needs to be done?")}
            bind:value={newDescription}
          ></textarea>
        </div>

        <div class="form-row" style="display:flex;gap:12px;margin-top:12px">
          <div class="form-group" style="flex:1">
            <label for="new-task-date" class="label">{__("Due Date")}</label>
            <input id="new-task-date" type="date" class="input" bind:value={newDate} />
          </div>
          <div class="form-group" style="flex:1">
            <label for="new-task-priority" class="label">{__("Priority")}</label>
            <select id="new-task-priority" class="input" bind:value={newPriority}>
              <option value="Low">{__("Low")}</option>
              <option value="Medium">{__("Medium")}</option>
              <option value="High">{__("High")}</option>
              <option value="Urgent">{__("Urgent")}</option>
            </select>
          </div>
        </div>

        <div class="form-group" style="margin-top:12px">
          <label for="new-task-assignee" class="label">{__("Assign to")}</label>
          <LinkControl
            id="new-task-assignee"
            field={userField}
            value={newAllocatedTo}
            onchange={(v) => (newAllocatedTo = v || "")}
          />
        </div>
      </div>
      <div class="foot">
        <button class="btn" onclick={() => (showNewModal = false)}>{__("Cancel")}</button>
        <button
          class="btn primary"
          disabled={!newDescription.trim() || savingNew}
          onclick={createStandaloneTask}
        >
          {savingNew ? __("Saving...") : __("Save")}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .page-head { display: flex; align-items: center; flex-wrap: wrap; gap: 8px; margin-bottom: 16px; }
  .page-head h1 { margin: 0 auto 0 0; }
  .view-switcher { display: flex; }
  .view-switcher .btn { border-radius: 0; }
  .view-switcher .btn:first-child { border-radius: var(--radius) 0 0 var(--radius); }
  .view-switcher .btn:last-child { border-radius: 0 var(--radius) var(--radius) 0; }
  .view-switcher .btn + .btn { margin-left: -1px; }
  .view-switcher .active { color: var(--primary); background: var(--bg); position: relative; border-color: var(--primary); }
  .toolbar { display: flex; align-items: center; justify-content: space-between; gap: 8px; flex-wrap: wrap; margin-bottom: 12px; }
  .toolbar > .btn.active { color: var(--primary); border-color: var(--primary); }
  .filter-count { margin-left: 2px; min-width: 18px; padding: 0 5px; border-radius: 999px; background: var(--primary); color: #fff; font-size: 11px; line-height: 18px; text-align: center; }
  .scope-tabs {
    display: inline-flex;
    background: var(--border, #e2e8f0);
    padding: 2px;
    border-radius: 6px;
  }
  .tab-btn {
    border: none;
    background: transparent;
    padding: 6px 14px;
    border-radius: 4px;
    font-size: 13px;
    font-weight: 500;
    color: var(--muted, #64748b);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .tab-btn.active {
    background: var(--surface, #fff);
    color: var(--text, #1e293b);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  }
  .list-filters { padding: 12px 14px; margin-bottom: 12px; display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px 10px; align-items: start; }
  .filter-search { grid-column: span 2; min-width: 0; }
  .filter-search label, .select-filter > label, .label-spacer { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
  .select-filter { min-width: 0; }
  .select-filter :global(.field) { flex: 1; }
  .list-results { overflow: auto; }
  /* below this the table scrolls sideways instead of squeezing its text columns */
  .list-results .grid { min-width: 860px; }
  .view-notice { margin: 0 0 8px; }
  .sort { border: 0; padding: 0; background: none; color: inherit; font: inherit; cursor: pointer; }
  .sort:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; }
  /* long text wraps between words; only an unbroken string is split */
  .desc { font-weight: 500; min-width: 200px; max-width: 360px; overflow-wrap: break-word; }
  .desc a { color: inherit; }
  tr.done .desc a { color: var(--muted); text-decoration: line-through; }
  .ref { min-width: 160px; max-width: 300px; overflow-wrap: break-word; }
  .who { min-width: 120px; }
  .nowrap { white-space: nowrap; }
  .overdue { color: var(--danger, #dc2626); font-weight: 600; }
  .todo-cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; margin-bottom: 12px; }
  .task-card { min-width: 0; display: flex; flex-direction: column; gap: 10px; padding: 14px 16px; }
  .task-card:hover, .task-card:focus-within { border-color: var(--primary); }
  .task-card header { display: flex; align-items: flex-start; gap: 10px; }
  .task-card header input { flex-shrink: 0; width: 16px; height: 16px; margin: 2px 0 0; cursor: pointer; }
  .task-card .desc { flex: 1; min-width: 0; max-width: none; color: var(--text); }
  .task-card.done .desc { color: var(--muted); text-decoration: line-through; }
  .task-card header .indicator { flex-shrink: 0; }
  .task-card .ref { min-width: 0; max-width: none; margin-left: 26px; font-size: 13px; }
  .task-card dl { display: grid; gap: 6px; margin: 0; }
  .task-card dl > div { display: flex; justify-content: space-between; gap: 12px; }
  .task-card dt { color: var(--muted); font-size: 12px; }
  .task-card dd { margin: 0; text-align: right; overflow-wrap: anywhere; min-width: 0; }
  .task-card footer { display: flex; justify-content: flex-end; margin-top: auto; padding-top: 10px; border-top: 1px solid var(--border); }
  .empty { text-align: center; padding: 36px 12px; color: var(--muted); }
  .empty p { margin: 6px 0 0; }
  .state { padding: 44px 20px; text-align: center; }
  .priority-badge {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 999px;
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    white-space: nowrap;
  }
  .priority-urgent { background: #fee2e2; color: #b91c1c; }
  .priority-high { background: #ffedd5; color: #c2410c; }
  .priority-medium { background: #fef3c7; color: #b45309; }
  .priority-low { background: #f1f5f9; color: #475569; }
  .task-actions { display: inline-flex; gap: 4px; white-space: nowrap; }
  .pagination { display: flex; align-items: center; gap: 8px; padding: 10px 14px; }
  .pagination .spacer { flex: 1; }
  .sr-only { position: absolute; width: 1px; height: 1px; overflow: hidden; clip: rect(0 0 0 0); white-space: nowrap; }
  .label {
    display: block;
    font-size: 12px;
    font-weight: 500;
    margin-bottom: 4px;
    color: var(--muted);
  }
  .label.reqd::after {
    content: " *";
    color: var(--danger, #e53e3e);
  }
  .form-group {
    display: flex;
    flex-direction: column;
  }
  @media (max-width: 800px) {
    .list-filters { grid-template-columns: 1fr; }
    .filter-search { grid-column: auto; }
  }
</style>
