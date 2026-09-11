<script lang="ts">
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { formatValue, formatCurrency, formatNumber, statusColor } from "$lib/format";
  import { getLinkTitle } from "$lib/titles.svelte";
  import Control from "$lib/controls/Control.svelte";
  import BarChart from "./BarChart.svelte";
  import Icon from "./Icon.svelte";
  import { showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { deskSDK } from "$lib/desk-sdk";
  import { toCsv, downloadCsv } from "$lib/csv";
  import { reportFiltersFromSearchParams, reportFiltersToSearchParams } from "./report-state";

  let { name }: { name: string } = $props();
  let meta = $state<any>(null);
  let result = $state<any>(null);
  let filters = $state<Record<string, any>>({});
  let loading = $state(false);
  let ready = false;
  let lastUrlSearch = "";
  const label = $derived(meta?.label || boot.data?.reports[name]?.label || name);

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
    goto(`/app/report/${encodeURIComponent(name)}${lastUrlSearch}`, { noScroll: true, keepFocus: true });
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
  const num = (c: any) => ["Int", "Float", "Currency", "Percent"].includes(c.fieldtype);
  const fmtSummary = (s: any) => (s.datatype === "Currency" ? formatCurrency(s.value) : s.datatype === "Int" ? String(s.value) : formatNumber(s.value));
  function exportCsv() {
    if (!result) return;
    const cols = result.columns;
    const rows = result.rows.map((r: any) => cols.map((c: any) => r[c.fieldname]));
    if (totals) rows.push(cols.map((c: any) => (num(c) ? totals![c.fieldname] : c === cols[0] ? __("Total") : "")));
    downloadCsv(`${name}.csv`, toCsv(cols.map((c: any) => c.label), rows));
  }
  // footer with the sum of every numeric column (Percent is an average, not a sum)
  const totals = $derived.by((): Record<string, number> | null => {
    if (!result?.rows?.length) return null;
    const cols = result.columns.filter((c: any) => num(c) && c.fieldtype !== "Percent");
    if (!cols.length) return null;
    const out: Record<string, number> = {};
    for (const c of cols) out[c.fieldname] = result.rows.reduce((a: number, r: any) => a + (Number(r[c.fieldname]) || 0), 0);
    return out;
  });
</script>

<div class="page">
  <div class="page-head"><h1>{label}</h1>{#if meta?.canExport !== false}<button class="btn" onclick={exportCsv}><Icon name="download" size={14} /> CSV</button>{/if}<button class="btn primary" onclick={run} disabled={loading}>{__("Update")}</button></div>
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
    <div class="summary">{#each result.summary as s}<div class="item {s.indicator || ''}"><div class="l">{s.label}</div><div class="v">{fmtSummary(s)}</div></div>{/each}</div>
  {/if}
  {#if result?.chart}
    <div class="card" style="padding:16px 20px;margin-bottom:12px"><BarChart data={result.chart} /></div>
  {/if}
  {#if result}
    <div class="card" style="overflow:auto">
      <table class="grid">
        <thead><tr>{#each result.columns as c}<th class:num={num(c)} style="min-width:{c.width || 100}px">{c.label}</th>{/each}</tr></thead>
        <tbody>
          {#each result.rows as r}
            <tr class="row">
              {#each result.columns as c}
                <td class:num={num(c)}>
                  {#if c.fieldtype === "Link" && r[c.fieldname]}
                    {@const linkTitle = getLinkTitle(c.options, r[c.fieldname]) || r[c.fieldname]}
                    <a href={`/app/${encodeURIComponent(c.options)}/${encodeURIComponent(r[c.fieldname])}`} title={r[c.fieldname]}>{linkTitle}</a>
                  {:else if c.fieldname === "status" && r[c.fieldname]}<span class="indicator {statusColor(r[c.fieldname], c)}">{__(r[c.fieldname])}</span>
                  {:else}<span style:color={c.fieldtype === "Currency" && r[c.fieldname] < 0 ? "var(--red)" : undefined}>{formatValue(r[c.fieldname], c)}</span>{/if}
                </td>
              {/each}
            </tr>
          {/each}
          {#if !result.rows.length}<tr><td colspan="30" class="empty">{__("No records")}</td></tr>{/if}
        </tbody>
        {#if totals}
          <tfoot>
            <tr class="totals">
              {#each result.columns as c, i}
                <td class:num={num(c)}>{#if totals[c.fieldname] !== undefined}{formatValue(totals[c.fieldname], c)}{:else if i === 0}{__("Total")}{/if}</td>
              {/each}
            </tr>
          </tfoot>
        {/if}
      </table>
    </div>
  {/if}
</div>

<style>
  .totals td { font-weight: 600; border-top: 2px solid var(--border); background: var(--bg-subtle, transparent); }
</style>
