<script lang="ts">
  // The table of a report's result: Link and status cells, a totals footer,
  // and optional sorting, row selection and CSV/XLSX export. Used by the
  // report page and by a form's Report field.
  import { __ } from "$lib/boot.svelte";
  import { formatValue, statusColor } from "$lib/format";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { isNumericFieldtype, type GridFilter } from "$lib/meta";
  import { downloadTable, exportTable, filterRows, nextSort, sortRows, type GridSortState } from "$lib/grid-rows";
  import GridExport from "$lib/controls/GridExport.svelte";
  import GridFilters from "$lib/controls/GridFilters.svelte";

  let {
    columns, rows, wsPrefix, filename, sheetName,
    baseSort = null, sortable = false, selectable = false, exportable = false, filters = [],
  }: {
    columns: any[]; rows: any[]; wsPrefix: string; filename: string; sheetName?: string;
    baseSort?: GridSortState | null; sortable?: boolean; selectable?: boolean; exportable?: boolean;
    filters?: GridFilter[];
  } = $props();

  const num = (c: any) => isNumericFieldtype(c.fieldtype);
  let userSort = $state<GridSortState | null>(null);
  const sort = $derived(userSort ?? baseSort);
  // the preset filters narrow the rows on screen; selection, totals and
  // export follow what is shown
  let activeFilters = $state<Set<number> | null>(null);
  const active = $derived(activeFilters ?? new Set(filters.flatMap((f, i) => (f.default ? [i] : []))));
  const visibleRows = $derived(filterRows(rows, [...active].map((i) => filters[i]?.filters).filter(Boolean), columns));
  const viewRows = $derived(sortRows(visibleRows, sort, columns, getLinkTitle));
  let selected = $state<Set<any>>(new Set());
  const chosen = $derived(visibleRows.filter((r) => selected.has(r)));
  const allSelected = $derived(visibleRows.length > 0 && chosen.length === visibleRows.length);
  function toggleFilter(i: number) { const s = new Set(active); if (s.has(i)) s.delete(i); else s.add(i); activeFilters = s; }

  // footer with the sum of every numeric column (Percent is an average, not a sum)
  const totals = $derived.by((): Record<string, number> | null => {
    if (!visibleRows.length) return null;
    const cols = columns.filter((c: any) => num(c) && c.fieldtype !== "Percent");
    if (!cols.length) return null;
    const out: Record<string, number> = {};
    for (const c of cols) out[c.fieldname] = visibleRows.reduce((a: number, r: any) => a + (Number(r[c.fieldname]) || 0), 0);
    return out;
  });

  function toggle(row: any) { const s = new Set(selected); if (s.has(row)) s.delete(row); else s.add(row); selected = s; }
  function toggleAll(on: boolean) { selected = on ? new Set(visibleRows) : new Set(); }
  function exportRows(format: "csv" | "xlsx") {
    const out = chosen.length ? viewRows.filter((r) => selected.has(r)) : viewRows;
    const { header, cells } = exportTable(out, columns, getLinkTitle);
    // the totals are of every row on screen, so they only go along with all of them
    if (totals && !chosen.length) cells.push(columns.map((c: any, i: number) => (totals[c.fieldname] !== undefined ? totals[c.fieldname] : i === 0 ? __("Total") : "")));
    downloadTable(format, filename, header, cells, sheetName);
  }
</script>

<div class="card" style="overflow:auto">
  {#if exportable || filters.length || (selectable && chosen.length)}
    <div class="grid-toolbar">
      {#if filters.length}<GridFilters {filters} {active} ontoggle={toggleFilter} />{/if}
      {#if selectable && chosen.length}
        <span class="muted">{__("{0} selected", [chosen.length])}</span>
        <button class="btn sm" onclick={() => toggleAll(false)}>{__("Clear selection")}</button>
      {/if}
      <span class="spacer"></span>
      {#if exportable}<GridExport onexport={exportRows} />{/if}
    </div>
  {/if}
  <table class="grid">
    <thead>
      <tr>
        {#if selectable}<th style="width:28px"><input type="checkbox" aria-label={__("Select all")} checked={allSelected} onchange={(e) => toggleAll(e.currentTarget.checked)} /></th>{/if}
        {#each columns as c}
          {@const sorted = sort?.field === c.fieldname ? sort!.order : null}
          <th class:num={num(c)} style="min-width:{c.width || 100}px" aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : sortable ? "none" : undefined}>
            {#if sortable}<button class="grid-sort" title={__("Sort by {0}", [c.label])} onclick={() => (userSort = nextSort(sort, c.fieldname, baseSort))}>{c.label}{#if sorted} {sorted === "asc" ? "↑" : "↓"}{/if}</button>{:else}{c.label}{/if}
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each viewRows as r, i}
        <tr class="row">
          {#if selectable}<td><input type="checkbox" aria-label={__("Select row {0}", [i + 1])} checked={selected.has(r)} onchange={() => toggle(r)} /></td>{/if}
          {#each columns as c}
            <td class:num={num(c)}>
              {#if c.fieldtype === "Link" && r[c.fieldname]}
                {@const linkTitle = getLinkTitle(c.options, r[c.fieldname]) || r[c.fieldname]}
                <a href={`${wsPrefix}/${encodeURIComponent(c.options)}/${encodeURIComponent(r[c.fieldname])}`} title={r[c.fieldname]}>{linkTitle}</a>
              {:else if c.fieldname === "status" && r[c.fieldname]}<span class="indicator {statusColor(r[c.fieldname], c)}">{__(r[c.fieldname])}</span>
              {:else}<span style:color={c.fieldtype === "Currency" && r[c.fieldname] < 0 ? "var(--red)" : undefined}>{formatValue(r[c.fieldname], c)}</span>{/if}
            </td>
          {/each}
        </tr>
      {/each}
      {#if !viewRows.length}<tr><td colspan={columns.length + (selectable ? 1 : 0) || 1} class="empty">{rows.length ? __("No rows match the filters") : __("No records")}</td></tr>{/if}
    </tbody>
    {#if totals}
      <tfoot>
        <tr class="totals">
          {#if selectable}<td></td>{/if}
          {#each columns as c, i}
            <td class:num={num(c)}>{#if totals[c.fieldname] !== undefined}{formatValue(totals[c.fieldname], c)}{:else if i === 0}{__("Total")}{/if}</td>
          {/each}
        </tr>
      </tfoot>
    {/if}
  </table>
</div>

<style>
  .totals td { font-weight: 600; border-top: 2px solid var(--border); background: var(--bg-subtle, transparent); }
</style>
