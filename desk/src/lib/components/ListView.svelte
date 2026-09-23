<script lang="ts">
  // List view generated from meta: standard filters, search, sort, paging, bulk delete.
  import { api } from "$lib/api";
  import { getMeta, selectLabels, selectOptions, type Meta, type Field, isLayout } from "$lib/meta";
  import { formatValue } from "$lib/format";
  import { __, boot, doctypeLabel } from "$lib/boot.svelte";
  import { showError, toast, confirm, dialog } from "$lib/ui.svelte";
  import { getLinkTitle, registerTitles } from "$lib/titles.svelte";
  import Control from "$lib/controls/Control.svelte";
  import Icon from "./Icon.svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { onMount, untrack } from "svelte";
  import { subscribe } from "$lib/events";
  import { coalesce } from "$lib/coalesce";
  import { toCsv, downloadCsv } from "$lib/csv";
  import { deskSDK, type DeskViewMode, type ListViewOptions } from "$lib/desk-sdk";
  import { fromDatetimeLocal, today } from "$lib/datetime";
  import { getCalendarDays } from "$lib/controls/date-format";
  import { buildListFilters } from "./list-filters";
  import { clearListFilters, listStateFromSearchParams, listStateToSearchParams, resolveAllowedViews, resolveActiveView, type ListUrlState } from "./list-state";
  import { exportChoice, exportChoices, exportUrl } from "./export-options";
  import DataImportModal from "./DataImportModal.svelte";
  import { canImport } from "./data-import";
  import TableView from "./views/TableView.svelte";
  import CardView from "./views/CardView.svelte";
  import CalendarView from "./views/CalendarView.svelte";
  import KanbanView from "./views/KanbanView.svelte";
  import GanttView from "./views/GanttView.svelte";
  import TreeView from "./views/TreeView.svelte";
  import { moveKanbanRow, kanbanValue } from "./views/kanban-state";
  import { ganttRangeFilters, ganttWindow, type GanttScale } from "./views/gantt-state";
  import { calendarRangeFilters } from "./views/calendar-state";
  import { resolveCardFields } from "./views/card-fields";
  import { showIDColumn } from "./views/id-column";

  let { doctype }: { doctype: string } = $props();
  let meta = $state<Meta | null>(null);
  let rows = $state<any[]>([]);
  let total = $state(0);
  let loading = $state(true);
  // a refused list is a state of the page, not a toast that fades: without
  // this the table below simply reads "No records", which is a different
  // thing from "you may not see these"
  let error = $state("");
  let filters = $state<Record<string, any>>({});
  let search = $state("");
  let orderBy = $state("");
  let pageSize = $state(20);
  let start = $state(0);
  let selected = $state<Set<string>>(new Set());
  let docstatusFilter = $state("");
  let timer: any;
  let ready = false;
  let lastUrlSearch = "";
  let isMobile = $state(false);
  let currentView = $state("list");
  let importOpen = $state(false);
  let loadVersion = 0;
  let loadedQueryKey = "";

  const initialDate = today();
  let calendarYear = $state(Number(initialDate.slice(0, 4)));
  let calendarMonth = $state(Number(initialDate.slice(5, 7)));
  const initialDays = getCalendarDays(Number(initialDate.slice(0, 4)), Number(initialDate.slice(5, 7)));
  let gridStartIso = $state(initialDays[0].iso);
  let gridEndIso = $state(initialDays[initialDays.length - 1].iso);

  let ganttScale = $state<GanttScale>("week");
  let ganttAnchor = $state(initialDate);
  /** Kanban and Gantt load one unpaginated window of rows, capped like the calendar. */
  const VIEW_LIMIT = 500;

  const workspace = $derived(page.params.workspace || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");

  // options registered by the app via defineListView(doctype, {...})
  const settings = $derived<ListViewOptions>(deskSDK.listSettings(doctype) || {});
  const allowedViews = $derived(resolveAllowedViews(settings, meta?.doctype.isTree));
  const isTreeView = $derived(currentView === "tree" && !!meta?.doctype.isTree);
  /** Bumped on a list_update so the tree reloads the branches it has open. */
  let treeReload = $state(0);

  function resolveView(urlView?: string | null) {
    let storedView: string | null = null;
    try { if (typeof window !== "undefined") storedView = window.localStorage.getItem(`ddcore_view_${doctype}`); } catch { /* Storage can be disabled by the browser. */ }
    return resolveActiveView(allowedViews, urlView, storedView, isMobile);
  }

  const columns = $derived.by(() => {
    if (!meta) return [] as Field[];
    // an explicit `columns` from defineListView wins over the meta's inListView
    const cols = settings.columns?.length
      ? (settings.columns.map((n) => meta!.doctype.fields.find((f) => f.fieldname === n)).filter(Boolean) as Field[])
      : meta.doctype.fields.filter((f) => f.inListView && !isLayout(f) && f.fieldtype !== "Table");
    if (!settings.columns?.length && meta.doctype.titleField && !cols.some((c) => c.fieldname === meta!.doctype.titleField)) {
      const tf = meta.doctype.fields.find((f) => f.fieldname === meta!.doctype.titleField);
      if (tf) cols.unshift(tf);
    }
    return settings.columns?.length ? cols : cols.slice(0, 7);
  });
  /** `defineListView({ docstatusFilter: false })` drops it where a `status` field already tells drafts apart. */
  const showDocstatusFilter = $derived(!!meta?.doctype.submittable && settings.docstatusFilter !== false);
  const stdFilters = $derived(meta ? meta.doctype.fields.filter((f) => f.inStandardFilter && !isLayout(f)) : []);
  const isDocTypeRef = (f: Field) =>
    f.fieldname === "ref_doctype" || f.fieldname === "reference_doctype" || (!!f.fieldname && f.fieldname.endsWith("_doctype"));

  const doctypeChoices = $derived.by(() => {
    const dts = boot.data?.doctypes || {};
    const names = Object.keys(dts);
    names.sort((a, b) => {
      const la = dts[a]?.label || a;
      const lb = dts[b]?.label || b;
      return la.localeCompare(lb);
    });
    return {
      options: names,
      optionLabels: names.map((n) => dts[n]?.label || n),
    };
  });

  function filterField(f: Field): Field {
    if (isDocTypeRef(f) && f.fieldtype === "Data") {
      return {
        ...f,
        fieldtype: "Select",
        options: doctypeChoices.options,
        optionLabels: doctypeChoices.optionLabels,
        reqd: false,
        readOnly: false,
        default: undefined,
      };
    }
    return { ...f, reqd: false, readOnly: false, default: undefined };
  }
  const statusField = $derived(meta?.doctype.fields.find((f) => f.fieldname === "status"));
  const urlFields = $derived(meta?.doctype.fields.filter((f) => f.fieldname && !isLayout(f)) || []);
  const hasActiveFilters = $derived(Object.values(filters).some((v) => v !== null && v !== undefined && v !== "") || !!search || (showDocstatusFilter && docstatusFilter !== ""));

  function defaultListState(): ListUrlState {
    return {
      filters: { ...(settings.filters || {}) }, search: "", docstatusFilter: "", orderBy: settings.orderBy || "", page: 1, pageSize: settings.pageSize || 20,
    };
  }
  function currentListState(): ListUrlState {
    return { filters: { ...filters }, search, docstatusFilter, orderBy, page: Math.floor(start / pageSize) + 1, pageSize, view: currentView };
  }
  function applyListState(state: ListUrlState) {
    filters = state.filters;
    search = state.search;
    docstatusFilter = state.docstatusFilter;
    orderBy = state.orderBy;
    pageSize = state.pageSize;
    start = (state.page - 1) * state.pageSize;
    currentView = resolveView(state.view);
  }
  function updateListState(state: ListUrlState) {
    applyListState(state);
    const params = listStateToSearchParams(state, urlFields);
    // An explicit List choice must also survive another browser's preference.
    if (state.view) params.set("view", state.view);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`${wsPrefix}/${encodeURIComponent(doctype)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
    load();
  }
  function setView(mode: DeskViewMode) {
    if (!allowedViews.includes(mode)) return;
    try { if (typeof window !== "undefined") window.localStorage.setItem(`ddcore_view_${doctype}`, mode); } catch { /* URL state still works without storage. */ }
    updateListState({ ...currentListState(), view: mode });
  }
  function changeMonth(year: number, month: number, startIso: string, endIso: string) {
    const changed = gridStartIso !== startIso || gridEndIso !== endIso;
    calendarYear = year;
    calendarMonth = month;
    gridStartIso = startIso;
    gridEndIso = endIso;
    if (changed && ready && currentView === "calendar") load();
  }
  function changeGantt(scale: GanttScale, anchor: string) {
    const changed = scale !== ganttScale || anchor !== ganttAnchor;
    ganttScale = scale;
    ganttAnchor = anchor;
    if (changed && ready && currentView === "gantt") load();
  }
  /**
   * Moves a Kanban card at once and saves the new column; a refusal (write
   * permission, a workflow, validation) puts the card back where it was.
   */
  async function moveCard(id: string, value: string) {
    const field = settings.kanban?.field;
    if (!field) return;
    const row = rows.find((r) => r.id === id);
    if (!row) return;
    const previous = kanbanValue(row, field);
    rows = moveKanbanRow(rows, id, field, value);
    try {
      await api.update(doctype, id, { [field]: value === "" ? null : value, modified: row.modified });
    } catch (e) {
      rows = moveKanbanRow(rows, id, field, previous);
      showError(e);
    }
  }
  let filterTimer: any;
  function updateFilter(name: string, value: any, debounce = false) {
    const next = { ...filters };
    if (value === null || value === undefined || value === "") delete next[name];
    else next[name] = value;
    if (debounce) {
      applyListState({ ...currentListState(), filters: next, page: 1 });
      clearTimeout(filterTimer);
      filterTimer = setTimeout(() => {
        updateListState({ ...currentListState(), filters: next, page: 1 });
      }, 250);
    } else {
      updateListState({ ...currentListState(), filters: next, page: 1 });
    }
  }
  function clearFilter(name: string) { updateFilter(name, null); }

  function buildFilters() {
    const out: any[] = [];
    out.push(...buildListFilters(filters, settings.filterOptions));
    if (showDocstatusFilter && docstatusFilter !== "") out.push(["docstatus", "=", Number(docstatusFilter)]);
    if (currentView === "calendar" && settings.calendar?.field && meta?.doctype?.fields) {
      out.push(...calendarRangeFilters(settings.calendar.field, meta.doctype.fields, gridStartIso, gridEndIso));
    }
    if (currentView === "gantt" && settings.gantt?.startField && settings.gantt.endField && meta?.doctype?.fields) {
      out.push(...ganttRangeFilters(settings.gantt.startField, settings.gantt.endField, meta.doctype.fields, ganttWindow(ganttAnchor, ganttScale)));
    }
    return out;
  }
  function buildOr() {
    if (!search || !meta || !meta.doctype) return undefined;
    const linkCols = columns.filter((c) => c.fieldtype === "Link" && c.fieldname).map((c) => c.fieldname!);
    const linkFields = (meta.doctype.fields || [])
      .filter((f) => f.fieldtype === "Link" && (f.inListView || f.inStandardFilter))
      .map((f) => f.fieldname!);
    const fields = ["id", ...(meta.doctype.searchFields || []), meta.doctype.titleField, ...linkCols, ...linkFields].filter(Boolean) as string[];
    return [...new Set(fields)].map((f) => [f, "like", `%${search}%`]);
  }
  async function load() {
    if (!meta) return;
    if (isTreeView) {
      // the tree loads its own levels through /api/tree
      treeReload++;
      loading = false;
      return;
    }
    const version = ++loadVersion;
    loading = true;
    try {
      const cardInfo = resolveCardFields(meta.doctype, settings.card);
      const calendar = settings.calendar;
      const { kanban, gantt } = settings;
      const fields = ["id", "modified", "docstatus", "owner", ...columns.map((c) => c.fieldname!), ...(settings.fields || []),
        ...cardInfo.fetchFields,
        calendar?.field, calendar?.endField, calendar?.titleField || meta.doctype.titleField, calendar?.colorField,
        kanban?.field, kanban?.titleField, kanban?.subtitleField, kanban?.colorField,
        gantt?.startField, gantt?.endField, gantt?.titleField, gantt?.colorField, gantt?.progressField,
      ].filter((field): field is string => !!field);
      // Every requested Dynamic Link needs its sibling type field for both
      // the rendered link and the API's title resolution.
      for (const field of meta.doctype.fields) {
        if (field.fieldname && fields.includes(field.fieldname) && field.fieldtype === "Dynamic Link" && typeof field.options === "string") fields.push(field.options);
      }
      if (statusField && !fields.includes(statusField.fieldname!)) fields.push(statusField.fieldname!);
      const unpaged = (currentView === "calendar" && !!calendar?.field) || (currentView === "kanban" && !!kanban?.field)
        || (currentView === "gantt" && !!gantt?.startField && !!gantt?.endField);
      const order = currentView === "gantt" && gantt?.startField && !orderBy ? `${gantt.startField} asc` : orderBy || undefined;
      const query = { filters: buildFilters(), or_filters: buildOr(), fields: [...new Set(fields)], order_by: order, limit: unpaged ? VIEW_LIMIT : pageSize, start: unpaged ? 0 : start, with_count: true };
      // List and Cards share a query and selection. Normalize unordered query
      // parts so restoring the same filters from the URL also keeps selection.
      const queryKey = JSON.stringify({ ...query, doctype,
        filters: query.filters.map((filter) => JSON.stringify(filter)).sort(),
        or_filters: query.or_filters?.map((filter) => JSON.stringify(filter)).sort(),
        fields: [...query.fields].sort(),
      });
      const preserveSelection = queryKey === loadedQueryKey;
      if (!preserveSelection) selected = new Set();
      const res = await api.list(doctype, query);
      if (version !== loadVersion) return;
      rows = res.rows;
      total = res.count;
      if (res.titles) registerTitles(res.titles);
      selected = preserveSelection ? new Set(rows.filter((row) => selected.has(row.id)).map((row) => row.id)) : new Set();
      loadedQueryKey = queryKey;
      error = "";
    } catch (e: any) { if (version === loadVersion) { error = e.message; showError(e); } } finally { if (version === loadVersion) loading = false; }
  }
  $effect(() => {
    const search = page.url.search;
    if (!meta) return;
    untrack(() => {
      if (!ready || search === lastUrlSearch) return;
      lastUrlSearch = search;
      applyListState(listStateFromSearchParams(page.url.searchParams, urlFields, defaultListState()));
      load();
    });
  });
  // The cleanup must be registered synchronously: onMount ignores whatever an
  // async callback resolves to, so the subscription would outlive the view.
  onMount(() => {
    let alive = true;
    isMobile = window.innerWidth < 768;
    const onResize = () => {
      isMobile = window.innerWidth < 768;
      const next = resolveView(page.url.searchParams.get("view"));
      if (ready && next !== currentView) { currentView = next; load(); }
    };
    window.addEventListener("resize", onResize);
    // coalesced: a Data Import publishes one list_update per row
    const refresh = coalesce(load);
    const off = subscribe("list_update", (p: any) => { if (alive && p.doctype === doctype) refresh(); });
    (async () => {
      try {
        const m = await getMeta(doctype);
        if (!alive) return; // navigated away while the meta was loading
        meta = m;
        applyListState(listStateFromSearchParams(page.url.searchParams, urlFields, defaultListState()));
        lastUrlSearch = page.url.search;
        ready = true;
        await load();
      } catch (e: any) { if (alive) { error = e.message; loading = false; showError(e); } }
    })();
    return () => { alive = false; loadVersion++; off(); window.removeEventListener("resize", onResize); clearTimeout(timer); clearTimeout(filterTimer); };
  });
  function onSearch() { clearTimeout(timer); timer = setTimeout(() => updateListState({ ...currentListState(), search, page: 1 }), 250); }
  function sort(f: Field) {
    const cur = orderBy.split(" ");
    updateListState({ ...currentListState(), orderBy: cur[0] === f.fieldname && cur[1] === "asc" ? `${f.fieldname} desc` : `${f.fieldname} asc` });
  }
  function docstatusLabel(r: any) { return Number(r.docstatus) === 2 ? __("Cancelled") : Number(r.docstatus) === 1 ? __("Submitted") : __("Draft"); }
  /** The canonical status value — what colours and comparisons key on. */
  function statusOf(r: any) {
    if (statusField && r[statusField.fieldname!]) return r[statusField.fieldname!];
    if (meta?.doctype.submittable) return Number(r.docstatus) === 2 ? "Cancelled" : Number(r.docstatus) === 1 ? "Submitted" : "Draft";
    return "";
  }
  /** The same status as the reader sees it. */
  function statusLabelOf(r: any) {
    if (statusField && r[statusField.fieldname!]) {
      const opts = selectOptions(statusField);
      const i = opts.indexOf(r[statusField.fieldname!]);
      return i >= 0 ? selectLabels(statusField)[i] : __(r[statusField.fieldname!]);
    }
    if (meta?.doctype.submittable) return docstatusLabel(r);
    return "";
  }
  function toggle(id: string) { const s = new Set(selected); s.has(id) ? s.delete(id) : s.add(id); selected = s; }
  async function deleteSelected() {
    if (!(await confirm(__("Delete {0} record(s)?", [selected.size])))) return;
    let ok = 0;
    for (const n of selected) { try { await api.remove(doctype, n); ok++; } catch (e) { showError(e); } }
    toast(__("{0} deleted", [ok]), { indicator: "green" });
    load();
  }
  /**
   * Asks what to export before exporting it. The page on screen is built here
   * (it already has the Link titles resolved); everything the filters match is
   * streamed by the server, which is the only way it can include rows this
   * page never loaded.
   */
  function openExport() {
    const all = total > rows.length;
    dialog({
      title: __("Export"),
      size: "sm",
      fields: [
        {
          // Scope and format are one field, not two. A dialog has no
          // conditional fields, and the pairs are not free: the page on
          // screen is a table of the columns already loaded, so it can only
          // be a CSV, and only NDJSON can nest the child tables. Listing the
          // combinations that exist is what stops the dialog from taking a
          // format it would then have to ignore.
          fieldname: "what", fieldtype: "Select", label: __("Export"),
          options: exportChoices,
          optionLabels: [
            __("The page on screen ({0}) as CSV", [rows.length]),
            __("All rows matching the filters ({0}) as CSV", [total]),
            __("All rows matching the filters ({0}) as NDJSON", [total]),
            __("All rows matching the filters ({0}) as NDJSON with the child tables", [total]),
          ],
          default: all ? "all-csv" : "page-csv",
        },
      ],
      primaryLabel: __("Export"),
      primaryAction(v, d) {
        d.hide();
        const choice = exportChoice(String(v.what));
        if (!choice) { exportLoadedPage(); return; }
        window.location.href = exportUrl({
          doctype,
          format: choice.format,
          children: choice.children,
          filters: buildFilters(),
          orFilters: buildOr(),
        });
      },
    }).show();
  }

  /** Exports the *loaded page* (not the whole result set) as CSV. */
  function exportLoadedPage() {
    const withID = showIDColumn(meta!.doctype, columns, settings);
    const keys = [...(withID ? ["id"] : []), ...columns.map((c) => c.fieldname!)];
    const header = [...(withID ? [meta!.doctype.idLabel || __("ID")] : []), ...columns.map((c) => c.label)];
    const offset = withID ? 1 : 0;
    const exportRows = rows.map((r) => keys.map((k, idx) => {
      if (idx < offset) return r[k];
      const col = columns[idx - offset];
      if (col && (col.fieldtype === "Link" || col.fieldtype === "Dynamic Link") && r[k]) {
        const target = col.fieldtype === "Dynamic Link" ? r[col.options] : col.options;
        return getLinkTitle(target, r[k]) || r[k];
      }
      return r[k];
    }));
    downloadCsv(`${doctype}.csv`, toCsv(header, exportRows));
    if (total > rows.length) toast(__("Exported {0} of {1} rows (the current page)", [rows.length, total]), { indicator: "blue" });
  }
  function cellText(r: any, c: Field) {
    const fx = settings.formatters?.[c.fieldname!];
    if (fx) return fx(r[c.fieldname!], r);
    if (isDocTypeRef(c) && r[c.fieldname!]) return doctypeLabel(r[c.fieldname!]);
    return formatValue(r[c.fieldname!], c);
  }
</script>

{#if error}
  <div class="page">
    <div class="page-head"><h1>{meta?.doctype.label || doctypeLabel(doctype)}</h1></div>
    <div class="card empty">{error}</div>
  </div>
{:else}
<div class="page">
  <div class="page-head">
    <h1>{meta?.doctype.label || doctypeLabel(doctype)}</h1>
    {#if allowedViews.length > 1}
      <div class="view-switcher">
        {#if allowedViews.includes("tree")}<button class="btn icon" class:active={currentView === "tree"} aria-pressed={currentView === "tree"} onclick={() => setView("tree")} title={__("Tree")} aria-label={__("Tree")}><Icon name="list-tree" size={14} /></button>{/if}
        {#if allowedViews.includes("list")}<button class="btn icon" class:active={currentView === "list"} aria-pressed={currentView === "list"} onclick={() => setView("list")} title={__("List")} aria-label={__("List")}><Icon name="list" size={14} /></button>{/if}
        {#if allowedViews.includes("calendar")}<button class="btn icon" class:active={currentView === "calendar"} aria-pressed={currentView === "calendar"} onclick={() => setView("calendar")} title={__("Calendar")} aria-label={__("Calendar")}><Icon name="calendar" size={14} /></button>{/if}
        {#if allowedViews.includes("kanban")}<button class="btn icon" class:active={currentView === "kanban"} aria-pressed={currentView === "kanban"} onclick={() => setView("kanban")} title={__("Kanban")} aria-label={__("Kanban")}><Icon name="square-kanban" size={14} /></button>{/if}
        {#if allowedViews.includes("gantt")}<button class="btn icon" class:active={currentView === "gantt"} aria-pressed={currentView === "gantt"} onclick={() => setView("gantt")} title={__("Gantt")} aria-label={__("Gantt")}><Icon name="chart-gantt" size={14} /></button>{/if}
        {#if allowedViews.includes("cards")}<button class="btn icon" class:active={currentView === "cards"} aria-pressed={currentView === "cards"} onclick={() => setView("cards")} title={__("Cards")} aria-label={__("Cards")}><Icon name="layout-grid" size={14} /></button>{/if}
      </div>
    {/if}
    {#if selected.size && meta?.permissions.delete}<button class="btn danger" onclick={deleteSelected}><Icon name="trash" size={14} />{__("Delete")} ({selected.size})</button>{/if}
    <button class="btn" onclick={load} title={__("Update")}><Icon name="refresh-cw" size={14} /></button>
    {#if canImport(meta?.permissions)}<button class="btn" onclick={() => (importOpen = true)} title={__("Import")} aria-label={__("Import")}><Icon name="upload" size={14} /></button>{/if}
    {#if meta?.permissions.export}<button class="btn" onclick={openExport} title={__("Export")}><Icon name="download" size={14} /></button>{/if}
    {#if meta?.permissions.create}<a class="btn primary" href={`${wsPrefix}/${encodeURIComponent(doctype)}/new`}><Icon name="plus" size={14} />{__("New")}</a>{/if}
  </div>

  {#if isTreeView}
    <p class="muted small view-notice">{__("Filters and search apply to the list view")}</p>
  {:else}
  <div class="card list-filters">
    <div class="filter-search">
      <label for="list-search">{__("Search")}</label>
      <input id="list-search" class="input" placeholder={__("Search…")} bind:value={search} oninput={onSearch} />
    </div>
    {#if showDocstatusFilter}
      <div class="select-filter">
        <label for="docstatus-filter">{__("Document status")}</label>
        <select id="docstatus-filter" class="input" bind:value={docstatusFilter} onchange={() => updateListState({ ...currentListState(), docstatusFilter, page: 1 })}>
          <option value="">{__("All")}</option><option value="0">{__("Draft")}</option><option value="1">{__("Submitted")}</option><option value="2">{__("Cancelled")}</option>
        </select>
        {#if docstatusFilter !== ""}<button class="btn icon filter-clear" onclick={() => updateListState({ ...currentListState(), docstatusFilter: "", page: 1 })} title={__("Remove the status filter")} aria-label={__("Remove the status filter")}><Icon name="x" size={14} /></button>{/if}
      </div>
    {/if}
    {#each stdFilters as f (f.fieldname)}
      {@const ff = filterField(f)}
      <div class="select-filter" class:labelled={!!ff.label}>
        {#if ff.fieldtype === "Check"}<span class="label-spacer" aria-hidden="true">&nbsp;</span>{/if}
        <Control field={ff} value={filters[f.fieldname!]} onchange={(v) => updateFilter(f.fieldname!, v, ff.fieldtype === "Data")} compact extraOptions={settings.filterOptions?.[f.fieldname!] || []} />
        {#if (ff.fieldtype === "Select" || ff.fieldtype === "Data") && filters[f.fieldname!] !== null && filters[f.fieldname!] !== undefined && filters[f.fieldname!] !== ""}
          <button class="btn icon filter-clear" onclick={() => clearFilter(f.fieldname!)} title={__("Remove the {0} filter", [ff.label])} aria-label={__("Remove the {0} filter", [ff.label])}><Icon name="x" size={14} /></button>
        {/if}
      </div>
    {/each}
    <div class="filter-actions">
      <span class="label-spacer" aria-hidden="true">&nbsp;</span>
      <button class="btn" disabled={!hasActiveFilters} onclick={() => updateListState(clearListFilters(currentListState()))}><Icon name="x" size={14} />{__("Clear filters")}</button>
    </div>
  </div>
  {/if}

  {#if meta}
    {#if isTreeView}
      <TreeView {meta} {doctype} {wsPrefix} reloadKey={treeReload} settings={settings.tree} />
    {:else if currentView === "calendar" && settings.calendar}
      <CalendarView {rows} {meta} {doctype} {wsPrefix} calendar={settings.calendar} viewYear={calendarYear} viewMonth={calendarMonth} onMonthChange={changeMonth} />
    {:else if currentView === "kanban" && settings.kanban}
      {#if total > rows.length}<div class="view-notice muted small">{__("Showing the first {0} of {1} records; narrow the filters to see the rest", [rows.length, total])}</div>{/if}
      <KanbanView {rows} {meta} {doctype} {wsPrefix} {loading} kanban={settings.kanban} onMove={moveCard} />
    {:else if currentView === "gantt" && settings.gantt}
      {#if total > rows.length}<div class="view-notice muted small">{__("Showing the first {0} of {1} records; narrow the filters to see the rest", [rows.length, total])}</div>{/if}
      <GanttView {rows} {meta} {doctype} {wsPrefix} gantt={settings.gantt} scale={ganttScale} anchor={ganttAnchor} onChange={changeGantt} />
    {:else}
      <div class:card={currentView !== "cards"} class="list-results">
        {#if currentView === "cards"}
          <CardView {rows} {meta} {doctype} {wsPrefix} {selected} {settings} {cellText} {loading} onToggle={toggle} />
        {:else}
          <TableView {rows} {meta} {doctype} {wsPrefix} {columns} {selected} {orderBy} {loading} {settings} {cellText} {statusOf} {statusLabelOf} onSort={sort} onToggle={toggle} onSelectAll={(checked) => selected = checked ? new Set(rows.map((r) => r.id)) : new Set()} />
        {/if}
        <div class="pagination" class:card={currentView === "cards"}>
          <span class="muted small">{total} {__("records")}</span>
          <span class="spacer"></span>
          <select class="input" style="width:auto" bind:value={pageSize} onchange={() => updateListState({ ...currentListState(), pageSize: Number(pageSize), page: 1 })}>{#each [20, 50, 100, 500] as n}<option value={n}>{n}</option>{/each}</select>
          <button class="btn sm" disabled={start === 0} onclick={() => updateListState({ ...currentListState(), page: Math.max(1, Math.floor(start / pageSize)) })}><Icon name="chevron-left" size={14} /></button>
          <span class="small muted">{Math.floor(start / pageSize) + 1} / {Math.max(1, Math.ceil(total / pageSize))}</span>
          <button class="btn sm" disabled={start + pageSize >= total} onclick={() => updateListState({ ...currentListState(), page: Math.floor(start / pageSize) + 2 })}><Icon name="chevron-right" size={14} /></button>
        </div>
      </div>
    {/if}
  {/if}
</div>
{/if}

<DataImportModal open={importOpen} {doctype} label={meta?.doctype.label || doctypeLabel(doctype)} permissions={meta?.permissions}
  onclose={() => (importOpen = false)} onimported={load} />

<style>
  .view-switcher { display: flex; }
  .view-switcher .btn { border-radius: 0; }
  .view-switcher .btn:first-child { border-radius: var(--radius) 0 0 var(--radius); }
  .view-switcher .btn:last-child { border-radius: 0 var(--radius) var(--radius) 0; }
  .view-switcher .btn + .btn { margin-left: -1px; }
  .view-switcher .active { color: var(--primary); background: var(--bg); position: relative; border-color: var(--primary); }
  .list-results { overflow: auto; }
  .view-notice { margin: 0 0 8px; }
  .pagination { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-top: 1px solid var(--border); }
  .pagination.card { margin-top: 12px; }
  .list-filters { padding: 12px 14px; margin-bottom: 12px; display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px 10px; align-items: start; }
  .filter-search { grid-column: span 2; min-width: 0; }
  .filter-search label, .select-filter > label, .label-spacer { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
  /* Controls with no label above them (Check, the clear button) get a blank spacer the height of a label, so they line up with the inputs. */
  .label-spacer { width: 100%; }
  .select-filter { display: flex; flex-wrap: wrap; column-gap: 4px; row-gap: 0; align-items: flex-start; min-width: 0; }
  .select-filter > label { width: 100%; }
  .select-filter > .input { flex: 1; width: auto; }
  .select-filter :global(.field) { flex: 1; }
  .filter-clear { flex: 0 0 auto; }
  .select-filter.labelled .filter-clear { margin-top: 20px; }
  .filter-actions { align-self: start; }
  @media (max-width: 800px) {
    .list-filters { grid-template-columns: 1fr; }
    .filter-search { grid-column: auto; }
  }
</style>
