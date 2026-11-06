<script lang="ts">
  import { seg, appPrefix } from "$lib/routes";
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { formatSummary } from "$lib/format";
  import Control from "$lib/controls/Control.svelte";
  import Chart from "./Chart.svelte";
  import ReportGrid from "./ReportGrid.svelte";
  import { showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
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
  const canDrill = (drill: unknown) => isDrill(drill, filterFields);
  function drill(d: unknown) {
    const next = drillFilters(filters, d, filterFields);
    if (!next) return;
    const params = reportFiltersToSearchParams(next, filterFields);
    goto(`${wsPrefix}/report/${seg(name)}${params.size ? `?${params}` : ""}`, { noScroll: true });
  }
  function updateFilter(fieldname: string, value: any) {
    filters = { ...filters, [fieldname]: value };
    const params = reportFiltersToSearchParams(filters, meta?.filters || []);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`${wsPrefix}/report/${seg(name)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
    run();
  }
  $effect(() => {
    const search = page.url.search;
    if (!ready || !meta || search === lastUrlSearch) return;
    lastUrlSearch = search;
    applyFiltersFromUrl();
    run();
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
  {#if meta?.filters?.length}
    <div class="card" style="padding:12px 14px;margin-bottom:12px;display:flex;gap:10px;flex-wrap:wrap;align-items:flex-end">
      {#each meta.filters as f (f.fieldname)}
        <div style="width:{f.fieldtype === 'Check' ? 'auto' : '190px'}">
          <Control field={f} value={filters[f.fieldname]} onchange={(v) => updateFilter(f.fieldname, v)} compact />
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
        <div class="card chart-card">
          <Chart data={c} pickable={(i) => canDrill(c.drill?.[i])} onpick={c.drill?.some(canDrill) ? (i) => drill(c.drill[i]) : undefined} />
        </div>
      {/each}
    </div>
  {/if}
  {#if result}
    <ReportGrid columns={result.columns} rows={result.rows} {wsPrefix} filename={name} sheetName={label} sortable exportable={meta?.canExport !== false}
      reportTotals={result.totals} rowclickable={(r) => canDrill(r.drill)} onrowclick={result.rows?.some((r: any) => canDrill(r.drill)) ? (r) => drill(r.drill) : undefined} />
  {/if}
</div>

<style>
  .page-title { flex: 1; min-width: 200px; }
  .subtitle { margin: 2px 0 0; font-size: 13px; }
  .charts { display: grid; gap: 12px; margin-bottom: 12px; }
  .charts.pair { grid-template-columns: repeat(auto-fit, minmax(min(100%, 420px), 1fr)); }
  .chart-card { padding: 16px 20px; min-width: 0; }
</style>
