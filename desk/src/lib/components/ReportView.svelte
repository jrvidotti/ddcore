<script lang="ts">
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { formatSummary } from "$lib/format";
  import Control from "$lib/controls/Control.svelte";
  import BarChart from "./BarChart.svelte";
  import ReportGrid from "./ReportGrid.svelte";
  import { showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { deskSDK } from "$lib/desk-sdk";
  import { reportFiltersFromSearchParams, reportFiltersToSearchParams } from "./report-state";
  import { getRememberedWorkspace } from "./sidebar-workspace";

  let { name }: { name: string } = $props();
  let meta = $state<any>(null);
  let result = $state<any>(null);
  let filters = $state<Record<string, any>>({});
  let loading = $state(false);
  let ready = false;
  let lastUrlSearch = "";
  const label = $derived(meta?.label || boot.data?.reports[name]?.label || name);
  const workspace = $derived(page.params.workspace || getRememberedWorkspace() || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");

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
  function updateFilter(fieldname: string, value: any) {
    filters = { ...filters, [fieldname]: value };
    const params = reportFiltersToSearchParams(filters, meta?.filters || []);
    lastUrlSearch = params.size ? `?${params}` : "";
    goto(`${wsPrefix}/report/${encodeURIComponent(name)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
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
  <div class="page-head"><h1>{label}</h1><button class="btn primary" onclick={run} disabled={loading}>{__("Update")}</button></div>
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
  {#if result?.chart}
    <div class="card" style="padding:16px 20px;margin-bottom:12px"><BarChart data={result.chart} /></div>
  {/if}
  {#if result}
    <ReportGrid columns={result.columns} rows={result.rows} {wsPrefix} filename={name} sheetName={label} sortable exportable={meta?.canExport !== false} />
  {/if}
</div>
