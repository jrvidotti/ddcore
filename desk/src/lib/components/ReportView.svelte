<script lang="ts">
  import { seg, appPrefix } from "$lib/routes";
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { formatSummary } from "$lib/format";
  import Control from "$lib/controls/Control.svelte";
  import Chart from "./Chart.svelte";
  import { flatDrill } from "./chart-layout";
  import ReportGrid from "./ReportGrid.svelte";
  import { showError } from "$lib/ui.svelte";
  import { onMount, untrack } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { deskSDK } from "$lib/desk-sdk";
  import { drillFilters, isDrill, reportFiltersFromSearchParams, reportFiltersToSearchParams } from "./report-state";
  import { getRememberedWorkspace } from "./sidebar-workspace";

  let { name }: { name: string } = $props();
  let meta = $state<any>(null);
  let result = $state<any>(null);
  let filters = $state<Record<string, any>>({});
  let loading = $state(false);
  let ready = false;
  let lastUrlSearch = "";
  // `chart` first, then the ones in `charts`
  const charts = $derived([result?.chart, ...(result?.charts ?? [])].filter((c) => c?.labels));
  // a chart's drill-downs by the index its marks hand over: a heatmap's, one per row and column,
  // laid out by cell (row * labels + col)
  const drillsOf = (c: any): unknown[] =>
    c.type === "heatmap" ? flatDrill(c.drill, c.datasets?.length ?? 0, c.labels.length) : (c.drill ?? []);
  const label = $derived(meta?.label || boot.data?.reports[name]?.label || name);
  const wsPrefix = $derived(appPrefix(page.params.workspace || getRememberedWorkspace()));

  function defaultFor(f: any) {
    const d = f.default;
    const dt = deskSDK.ddcore.datetime;
    if (d === "Today") return dt.today();
    if (d === "month_end") return dt.monthEnd();
    if (d === "month_start") return dt.monthStart();
    if (typeof d === "string" && /^-?\d+m$/.test(d)) return dt.monthStart(dt.addMonths(dt.today(), parseInt(d)));
    return d;
  }
  async function run() {
    loading = true;
    try {
      const r = await api.report(name, filters);
      meta = r.meta;
      result = r.result;
    } catch (e) { showError(e); } finally { loading = false; }
  }
  function defaultFilters() {
    const defaults: Record<string, any> = {};
    for (const f of meta?.filters || []) if (f.fieldname && f.default !== undefined) defaults[f.fieldname] = defaultFor(f);
    return defaults;
  }
  function applyFiltersFromUrl() {
    filters = reportFiltersFromSearchParams(page.url.searchParams, meta?.filters || [], defaultFilters());
  }
  // A row's or a chart label's `drill` reopens the report with those filters over the current
  // ones. It goes through the URL, so Back returns to the cut the user drilled from; the URL
  // effect below applies the filters and runs the report.
  const filterFields = $derived(meta?.filters || []);
  // A `placement: "table"` filter is drawn in the table's toolbar. Until a result draws the
  // table (a first run that failed), it stays in the card, so it can still be changed.
  const inTable = (f: any) => f.placement === "table" && !!result;
  const cardFilters = $derived(filterFields.filter((f: any) => !inTable(f)));
  const tableFilters = $derived(filterFields.filter(inTable));
  const canDrill = (drill: unknown) => isDrill(drill, filterFields);
  // The table keeps its tallest height while only its own filters change: a grouping with fewer
  // rows would shorten the page and pull the scroll up under the reader. A change to the cut (a
  // card filter, a drill) lets it go; another report is a new view (the route's {#key name}).
  let gridBox = $state<HTMLElement>();
  let holdHeight = $state(0);
  function holdTable() {
    const h = (gridBox?.firstElementChild as HTMLElement | null)?.offsetHeight ?? 0;
    holdHeight = Math.max(holdHeight, h);
  }
  const onlyTableChanged = (before: Record<string, any>, after: Record<string, any>) =>
    filterFields.every((f: any) => inTable(f) || (before[f.fieldname] ?? null) === (after[f.fieldname] ?? null));
  function drill(d: unknown) {
    const next = drillFilters(filters, d, filterFields);
    if (!next) return;
    const params = reportFiltersToSearchParams(next, filterFields);
    goto(`${wsPrefix}/report/${seg(name)}${params.size ? `?${params}` : ""}`, { noScroll: true });
  }
  function updateFilter(fieldname: string, value: any) {
    const next = { ...filters, [fieldname]: value };
    if (onlyTableChanged(filters, next)) holdTable();
    else holdHeight = 0;
    filters = next;
    const params = reportFiltersToSearchParams(filters, meta?.filters || []);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`${wsPrefix}/report/${seg(name)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
    run();
  }
  $effect(() => {
    const search = page.url.search;
    if (!ready || !meta || search === lastUrlSearch) return;
    lastUrlSearch = search;
    untrack(() => {
      const before = filters;
      applyFiltersFromUrl();
      // Back over a change of grouping is still the same cut
      if (onlyTableChanged(before, filters)) holdTable();
      else holdHeight = 0;
      run();
    });
  });
  onMount(async () => {
    try {
      const r = await api.report(name, {});
      meta = r.meta;
      applyFiltersFromUrl();
      lastUrlSearch = page.url.search;
      ready = true;
      await run();
    } catch (e) { showError(e); }
  });
</script>

<div class="page">
  <div class="page-head">
    <div class="page-title">
      <h1>{label}</h1>
      {#if meta?.description}<p class="subtitle muted">{meta.description}</p>{/if}
    </div>
    <button class="btn primary" onclick={run} disabled={loading}>{__("Update")}</button>
  </div>
  {#if cardFilters.length}
    <div class="card filters">
      {#each cardFilters as f (f.fieldname)}
        <div class="filter" class:check={f.fieldtype === "Check"} class:unlabelled={f.fieldtype === "Check" || !f.label || f.hideLabel}>
          <Control field={f} value={filters[f.fieldname]} onchange={(v) => updateFilter(f.fieldname, v)} doc={filters} compact />
        </div>
      {/each}
    </div>
  {/if}
  {#if result?.summary?.length}
    <div class="summary">{#each result.summary as s}<div class="item {s.indicator || ''}"><div class="l">{s.label}</div><div class="v">{formatSummary(s)}</div></div>{/each}</div>
  {/if}
  {#if charts.length}
    <div class="charts" class:pair={charts.length > 1}>
      {#each charts as c}
        {@const drills = drillsOf(c)}
        <div class="card chart-card">
          <Chart data={c} pickable={(i) => canDrill(drills[i])} onpick={drills.some(canDrill) ? (i) => drill(drills[i]) : undefined} />
        </div>
      {/each}
    </div>
  {/if}
  {#snippet tableControls()}
    {#each tableFilters as f (f.fieldname)}
      {#if f.fieldtype === "Check"}
        <div class="table-filter"><Control field={f} value={filters[f.fieldname]} onchange={(v) => updateFilter(f.fieldname, v)} doc={filters} compact /></div>
      {:else}
        <!-- the label beside the input, as a toolbar reads; the control keeps its own for screen readers -->
        <div class="table-filter">
          <span class="table-filter-label" aria-hidden="true">{f.label}</span>
          <Control field={{ ...f, hideLabel: true, description: undefined }} value={filters[f.fieldname]} onchange={(v) => updateFilter(f.fieldname, v)} doc={filters} compact />
        </div>
      {/if}
    {/each}
  {/snippet}
  {#if result}
    <div bind:this={gridBox}>
    <ReportGrid minHeight={holdHeight} controls={tableFilters.length ? tableControls : undefined} columns={result.columns} rows={result.rows} {wsPrefix} filename={name} sheetName={label} sortable exportable={meta?.canExport !== false}
      reportTotals={result.totals} rowclickable={(r) => canDrill(r.drill)} onrowclick={result.rows?.some((r: any) => canDrill(r.drill)) ? (r) => drill(r.drill) : undefined} />
    </div>
  {/if}
</div>

<style>
  .page-title { flex: 1; min-width: 200px; }
  .subtitle { margin: 2px 0 0; font-size: 13px; }
  .charts { display: grid; gap: 12px; margin-bottom: 12px; }
  .charts.pair { grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr)); }
  .chart-card { padding: 16px 20px; min-width: 0; }
  /* Top-aligned, as in a form: labels on one line, inputs on one line, and a filter's
     description or error below its own input without lifting it above its neighbours. */
  .filters { padding: 12px 14px; margin-bottom: 12px; display: flex; gap: 10px; flex-wrap: wrap; align-items: flex-start; }
  .filter { width: 190px; }
  .filter.check { width: auto; }
  /* A filter with no label above its input (a Check carries its label beside the box) takes a
     label's height first, so its control sits on the inputs' line. */
  .filter.unlabelled::before { content: "\200b"; display: block; font-size: 12px; margin-bottom: 4px; }
  /* a filter in the table's toolbar: its label beside a toolbar-sized input */
  .table-filter { display: flex; align-items: center; gap: 6px; }
  .table-filter + .table-filter { margin-left: 6px; }
  .table-filter-label { color: var(--muted); white-space: nowrap; }
  .table-filter :global(input:not([type="checkbox"])), .table-filter :global(select) { width: 150px; min-height: 28px; padding: 3px 8px; font-size: 12px; }
</style>
