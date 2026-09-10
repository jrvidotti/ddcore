<script lang="ts">
  // List view generated from meta: standard filters, search, sort, paging, bulk delete.
  import { api } from "$lib/api";
  import { getMeta, type Meta, type Field, isLayout } from "$lib/meta";
  import { formatValue, statusColor, timeAgo } from "$lib/format";
  import { __, doctypeLabel } from "$lib/boot.svelte";
  import { showError, toast, confirm } from "$lib/ui.svelte";
  import { getLinkTitle, registerTitles } from "$lib/titles.svelte";
  import Control from "$lib/controls/Control.svelte";
  import Icon from "./Icon.svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { onMount } from "svelte";
  import { subscribe } from "$lib/events";
  import { toCsv, downloadCsv } from "$lib/csv";
  import { deskSDK } from "$lib/desk-sdk";
  import { clearListFilters, listStateFromSearchParams, listStateToSearchParams, type ListUrlState } from "./list-state";

  let { doctype }: { doctype: string } = $props();
  let meta = $state<Meta | null>(null);
  let rows = $state<any[]>([]);
  let total = $state(0);
  let loading = $state(true);
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

  // options registered by the app via defineListView(doctype, {...})
  interface ListSettings {
    columns?: string[];
    filters?: Record<string, any>;
    orderBy?: string;
    pageSize?: number;
    formatters?: Record<string, (value: any, row: any) => string>;
    indicator?: (row: any) => { label: string; color: string } | null | undefined;
  }
  const settings = $derived<ListSettings>(deskSDK.listSettings(doctype) || {});

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
  const stdFilters = $derived(meta ? meta.doctype.fields.filter((f) => f.inStandardFilter && !isLayout(f)) : []);
  const statusField = $derived(meta?.doctype.fields.find((f) => ["status", "situacao"].includes(f.fieldname || "")));
  const urlFields = $derived(meta?.doctype.fields.filter((f) => f.fieldname && !isLayout(f)) || []);
  const hasActiveFilters = $derived(Object.values(filters).some((v) => v !== null && v !== undefined && v !== "") || !!search || docstatusFilter !== "");

  function defaultListState(): ListUrlState {
    return {
      filters: { ...(settings.filters || {}) }, search: "", docstatusFilter: "", orderBy: settings.orderBy || "", page: 1, pageSize: settings.pageSize || 20,
    };
  }
  function currentListState(): ListUrlState {
    return { filters: { ...filters }, search, docstatusFilter, orderBy, page: Math.floor(start / pageSize) + 1, pageSize };
  }
  function applyListState(state: ListUrlState) {
    filters = state.filters;
    search = state.search;
    docstatusFilter = state.docstatusFilter;
    orderBy = state.orderBy;
    pageSize = state.pageSize;
    start = (state.page - 1) * state.pageSize;
  }
  function updateListState(state: ListUrlState) {
    applyListState(state);
    const params = listStateToSearchParams(state, urlFields);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`/app/${encodeURIComponent(doctype)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
    load();
  }
  function updateFilter(name: string, value: any) {
    const next = { ...filters };
    if (value === null || value === undefined || value === "") delete next[name];
    else next[name] = value;
    updateListState({ ...currentListState(), filters: next, page: 1 });
  }
  function clearFilter(name: string) { updateFilter(name, null); }

  function buildFilters() {
    const out: any[] = [];
    for (const [k, v] of Object.entries(filters)) if (v !== null && v !== undefined && v !== "") out.push([k, "=", v]);
    if (docstatusFilter !== "") out.push(["docstatus", "=", Number(docstatusFilter)]);
    return out;
  }
  function buildOr() {
    if (!search || !meta || !meta.doctype) return undefined;
    const linkCols = columns.filter((c) => c.fieldtype === "Link" && c.fieldname).map((c) => c.fieldname!);
    const linkFields = (meta.doctype.fields || [])
      .filter((f) => f.fieldtype === "Link" && (f.inListView || f.inStandardFilter))
      .map((f) => f.fieldname!);
    const fields = ["name", ...(meta.doctype.searchFields || []), meta.doctype.titleField, ...linkCols, ...linkFields].filter(Boolean) as string[];
    return [...new Set(fields)].map((f) => [f, "like", `%${search}%`]);
  }
  async function load() {
    if (!meta) return;
    loading = true;
    try {
      const fields = ["name", "modified", "docstatus", "owner", ...columns.map((c) => c.fieldname!)];
      if (statusField && !fields.includes(statusField.fieldname!)) fields.push(statusField.fieldname!);
      const res = await api.list(doctype, { filters: buildFilters(), or_filters: buildOr(), fields: [...new Set(fields)], order_by: orderBy || undefined, limit: pageSize, start, with_count: true });
      rows = res.rows;
      total = res.count;
      if (res.titles) registerTitles(res.titles);
      selected = new Set();
    } catch (e) { showError(e); } finally { loading = false; }
  }
  $effect(() => {
    const search = page.url.search;
    if (!ready || !meta || search === lastUrlSearch) return;
    lastUrlSearch = search;
    applyListState(listStateFromSearchParams(page.url.searchParams, urlFields, defaultListState()));
    load();
  });
  // The cleanup must be registered synchronously: onMount ignores whatever an
  // async callback resolves to, so the subscription would outlive the view.
  onMount(() => {
    let alive = true;
    const off = subscribe("list_update", (p: any) => { if (alive && p.doctype === doctype) load(); });
    (async () => {
      try {
        const m = await getMeta(doctype);
        if (!alive) return; // navigated away while the meta was loading
        meta = m;
        applyListState(listStateFromSearchParams(page.url.searchParams, urlFields, defaultListState()));
        lastUrlSearch = page.url.search;
        ready = true;
        await load();
      } catch (e) { if (alive) showError(e); }
    })();
    return () => { alive = false; off(); clearTimeout(timer); };
  });
  function onSearch() { clearTimeout(timer); timer = setTimeout(() => updateListState({ ...currentListState(), search, page: 1 }), 250); }
  function sort(f: Field) {
    const cur = orderBy.split(" ");
    updateListState({ ...currentListState(), orderBy: cur[0] === f.fieldname && cur[1] === "asc" ? `${f.fieldname} desc` : `${f.fieldname} asc` });
  }
  const num = (f: Field) => ["Int", "Float", "Currency", "Percent"].includes(f.fieldtype);
  function docstatusLabel(r: any) { return Number(r.docstatus) === 2 ? __("Cancelled") : Number(r.docstatus) === 1 ? __("Submitted") : __("Draft"); }
  function statusOf(r: any) {
    if (statusField && r[statusField.fieldname!]) return r[statusField.fieldname!];
    if (meta?.doctype.submittable) return docstatusLabel(r);
    return "";
  }
  function toggle(name: string) { const s = new Set(selected); s.has(name) ? s.delete(name) : s.add(name); selected = s; }
  async function deleteSelected() {
    if (!(await confirm(__("Delete {0} record(s)?", [selected.size])))) return;
    let ok = 0;
    for (const n of selected) { try { await api.remove(doctype, n); ok++; } catch (e) { showError(e); } }
    toast(__("{0} deleted", [ok]), { indicator: "green" });
    load();
  }
  /** Exports the *loaded page* (not the whole result set) as CSV. */
  function exportCsv() {
    const keys = ["name", ...columns.map((c) => c.fieldname!)];
    const header = [__("Name"), ...columns.map((c) => c.label)];
    const exportRows = rows.map((r) => keys.map((k, idx) => {
      if (idx === 0) return r[k];
      const col = columns[idx - 1];
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
    return fx ? fx(r[c.fieldname!], r) : formatValue(r[c.fieldname!], c);
  }
</script>

<div class="page">
  <div class="page-head">
    <h1>{meta?.doctype.label || doctypeLabel(doctype)}</h1>
    {#if selected.size && meta?.permissions.delete}<button class="btn danger" onclick={deleteSelected}><Icon name="trash" size={14} />{__("Delete")} ({selected.size})</button>{/if}
    <button class="btn" onclick={load} title={__("Update")}><Icon name="refresh-cw" size={14} /></button>
    {#if meta?.permissions.export}<button class="btn" onclick={exportCsv} title="CSV"><Icon name="download" size={14} /></button>{/if}
    {#if meta?.permissions.create}<a class="btn primary" href={`/app/${encodeURIComponent(doctype)}/new`}><Icon name="plus" size={14} />{__("New")}</a>{/if}
  </div>

  <div class="card list-filters">
    <div class="filter-search">
      <label for="list-search">{__("Search")}</label>
      <input id="list-search" class="input" placeholder={__("Search…")} bind:value={search} oninput={onSearch} />
    </div>
    {#if meta?.doctype.submittable}
      <div class="select-filter">
        <label for="docstatus-filter">{__("Document status")}</label>
        <select id="docstatus-filter" class="input" bind:value={docstatusFilter} onchange={() => updateListState({ ...currentListState(), docstatusFilter, page: 1 })}>
          <option value="">{__("All")}</option><option value="0">{__("Draft")}</option><option value="1">{__("Submitted")}</option><option value="2">{__("Cancelled")}</option>
        </select>
        {#if docstatusFilter !== ""}<button class="btn icon filter-clear" onclick={() => updateListState({ ...currentListState(), docstatusFilter: "", page: 1 })} title={__("Remove the status filter")} aria-label={__("Remove the status filter")}><Icon name="x" size={14} /></button>{/if}
      </div>
    {/if}
    {#each stdFilters as f (f.fieldname)}
      <div class="select-filter" class:labelled={!!f.label}>
        <Control field={{ ...f, reqd: false, readOnly: false, default: undefined }} value={filters[f.fieldname!]} onchange={(v) => updateFilter(f.fieldname!, v)} compact />
        {#if f.fieldtype === "Select" && filters[f.fieldname!] !== null && filters[f.fieldname!] !== undefined && filters[f.fieldname!] !== ""}
          <button class="btn icon filter-clear" onclick={() => clearFilter(f.fieldname!)} title={__("Remove the {0} filter", [f.label])} aria-label={__("Remove the {0} filter", [f.label])}><Icon name="x" size={14} /></button>
        {/if}
      </div>
    {/each}
    <div class="filter-actions">
      <button class="btn" disabled={!hasActiveFilters} onclick={() => updateListState(clearListFilters(currentListState()))}><Icon name="x" size={14} />{__("Clear filters")}</button>
    </div>
  </div>

  <div class="card" style="overflow:auto">
    <table class="grid">
      <thead>
        <tr>
          <th style="width:28px"><input type="checkbox" checked={rows.length > 0 && selected.size === rows.length} onchange={(e) => (selected = (e.target as HTMLInputElement).checked ? new Set(rows.map((r) => r.name)) : new Set())} /></th>
          {#if !columns.some((c) => c.fieldname === meta?.doctype.titleField) || meta?.doctype.naming?.field !== meta?.doctype.titleField}
            <th onclick={() => sort({ fieldname: "name", fieldtype: "Data" })} style="cursor:pointer">{__("Name")}</th>
          {/if}
          {#each columns as c}
            <th class:num={num(c)} onclick={() => sort(c)} style="cursor:pointer">{c.label} {#if orderBy.startsWith(c.fieldname + " ")}{orderBy.endsWith("asc") ? "↑" : "↓"}{/if}</th>
          {/each}
          {#if statusField || settings.indicator || meta?.doctype.submittable}<th>{__("Status")}</th>{/if}
          <th class="num">{__("Modified")}</th>
        </tr>
      </thead>
      <tbody>
        {#each rows as r (r.name)}
          <tr class="row" style="cursor:pointer" onclick={() => goto(`/app/${encodeURIComponent(doctype)}/${encodeURIComponent(r.name)}`)}>
            <td onclick={(e) => { e.stopPropagation(); toggle(r.name); }}><input type="checkbox" checked={selected.has(r.name)} onclick={(e) => e.stopPropagation()} onchange={() => toggle(r.name)} /></td>
            {#if !columns.some((c) => c.fieldname === meta?.doctype.titleField) || meta?.doctype.naming?.field !== meta?.doctype.titleField}
              <td><a href={`/app/${encodeURIComponent(doctype)}/${encodeURIComponent(r.name)}`} onclick={(e) => e.stopPropagation()}>{r.name}</a></td>
            {/if}
            {#each columns as c}
              <td class:num={num(c)} class:bold={c.bold} style:font-weight={c.bold ? 600 : undefined}>
                {#if c.fieldtype === "Link" && r[c.fieldname!]}
                  {@const linkTarget = c.options}
                  {@const linkVal = r[c.fieldname!]}
                  {@const linkTitle = getLinkTitle(linkTarget, linkVal)}
                  <a href={`/app/${encodeURIComponent(linkTarget)}/${encodeURIComponent(linkVal)}`} title={linkVal} onclick={(e) => e.stopPropagation()}>{linkTitle || linkVal}</a>
                {:else if c.fieldtype === "Dynamic Link" && r[c.fieldname!]}
                  {@const linkTarget = r[c.options]}
                  {@const linkVal = r[c.fieldname!]}
                  {@const linkTitle = getLinkTitle(linkTarget, linkVal)}
                  <a href={`/app/${encodeURIComponent(linkTarget)}/${encodeURIComponent(linkVal)}`} title={linkVal} onclick={(e) => e.stopPropagation()}>{linkTitle || linkVal}</a>
                {:else if c.fieldname === statusField?.fieldname}
                  <span class="indicator {statusColor(r[c.fieldname!])}">{r[c.fieldname!]}</span>
                {:else}
                  {cellText(r, c)}
                {/if}
              </td>
            {/each}
            {#if statusField || settings.indicator || meta?.doctype.submittable}
              <td>
                {#if settings.indicator}
                  {@const ind = settings.indicator(r)}
                  {#if ind}<span class="indicator {ind.color}">{ind.label}</span>{/if}
                {:else if statusOf(r)}<span class="indicator {statusColor(statusOf(r))}">{statusOf(r)}</span>{/if}
              </td>
            {/if}
            <td class="num muted small">{timeAgo(r.modified)}</td>
          </tr>
        {/each}
        {#if !loading && !rows.length}
          <tr><td colspan="20" class="empty">{__("No records")}</td></tr>
        {/if}
      </tbody>
    </table>
    <div style="display:flex;align-items:center;gap:8px;padding:10px 14px;border-top:1px solid var(--border)">
      <span class="muted small">{total} {__("records")}</span>
      <span class="spacer"></span>
      <select class="input" style="width:auto" bind:value={pageSize} onchange={() => updateListState({ ...currentListState(), pageSize: Number(pageSize), page: 1 })}>{#each [20, 50, 100, 500] as n}<option value={n}>{n}</option>{/each}</select>
      <button class="btn sm" disabled={start === 0} onclick={() => updateListState({ ...currentListState(), page: Math.max(1, Math.floor(start / pageSize)) })}><Icon name="chevron-left" size={14} /></button>
      <span class="small muted">{Math.floor(start / pageSize) + 1} / {Math.max(1, Math.ceil(total / pageSize))}</span>
      <button class="btn sm" disabled={start + pageSize >= total} onclick={() => updateListState({ ...currentListState(), page: Math.floor(start / pageSize) + 2 })}><Icon name="chevron-right" size={14} /></button>
    </div>
  </div>
</div>

<style>
  .list-filters { padding: 12px 14px; margin-bottom: 12px; display: grid; grid-template-columns: repeat(auto-fit, minmax(180px, 1fr)); gap: 12px 10px; align-items: start; }
  .filter-search { grid-column: span 2; min-width: 0; }
  .filter-search label, .select-filter > label { display: block; font-size: 12px; color: var(--muted); margin-bottom: 4px; }
  .select-filter { display: flex; flex-wrap: wrap; column-gap: 4px; row-gap: 0; align-items: flex-start; min-width: 0; }
  .select-filter > label { width: 100%; }
  .select-filter > .input { flex: 1; width: auto; }
  .select-filter :global(.field) { flex: 1; }
  .filter-clear { flex: 0 0 auto; }
  .select-filter.labelled .filter-clear { margin-top: 20px; }
  .filter-actions { align-self: end; }
  @media (max-width: 800px) {
    .list-filters { grid-template-columns: 1fr; }
    .filter-search { grid-column: auto; }
  }
</style>
