<script lang="ts">
  import { seg } from "$lib/routes";
  // The table of a report's result: Link and status cells, a totals footer,
  // and optional sorting, row selection and CSV/XLSX export. Used by the
  // report page and by a form's Report field.
  import { __ } from "$lib/boot.svelte";
  import { formatValue, isIndicatorSelect } from "$lib/format";
  import SelectIndicator from "$lib/components/SelectIndicator.svelte";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { isNumericFieldtype, type GridFilter } from "$lib/meta";
  import { downloadTable, exportTable, type GridSortState } from "$lib/grid-rows";
  import { GridRowsState } from "$lib/grid-rows-state.svelte";
  import type { FieldButton, GridAction } from "$lib/form.svelte";
  import FieldButtons from "$lib/controls/FieldButtons.svelte";
  import GridActions from "$lib/controls/GridActions.svelte";
  import GridExport from "$lib/controls/GridExport.svelte";
  import GridFilters from "$lib/controls/GridFilters.svelte";
  import GridSearch from "$lib/controls/GridSearch.svelte";
  import { forwardCellClick, isRowClick } from "$lib/grid-click";

  let {
    columns, rows, wsPrefix, filename, sheetName,
    baseSort = null, sortable = false, selectable = false, exportable = false, filters = [], search = [], buttons = [], actions = [],
    clickable = new Set(), oncellclick, onrowclick,
  }: {
    columns: any[]; rows: any[]; wsPrefix: string; filename: string; sheetName?: string;
    baseSort?: GridSortState | null; sortable?: boolean; selectable?: boolean; exportable?: boolean;
    filters?: GridFilter[];
    /** the columns the search box looks in; none, no box */
    search?: string[];
    /** a Report field's own actions (see frm.addFieldButton), shown in the toolbar */
    buttons?: FieldButton[];
    /** a Report field's actions on the selected rows (see frm.addGridAction) */
    actions?: GridAction[];
    /** the columns whose non-empty cells are buttons calling oncellclick (a Report field's grids.<field>.onCellClick) */
    clickable?: Set<string>;
    oncellclick?: (column: string, row: any) => void;
    /** a click on a row outside its links and cell buttons (a Report field's grids.<field>.onRowClick) */
    onrowclick?: (row: any) => void;
  } = $props();

  const num = (c: any) => isNumericFieldtype(c.fieldtype);
  const grid = new GridRowsState({ rows: () => rows, columns: () => columns, presets: () => filters, search: () => search, baseSort: () => baseSort });
  // the totals and export follow what is shown

  // footer with the sum of every numeric column (Percent is an average, not a sum)
  const totals = $derived.by((): Record<string, number> | null => {
    if (!grid.visibleRows.length) return null;
    const cols = columns.filter((c: any) => num(c) && c.fieldtype !== "Percent");
    if (!cols.length) return null;
    const out: Record<string, number> = {};
    for (const c of cols) out[c.fieldname] = grid.visibleRows.reduce((a: number, r: any) => a + (Number(r[c.fieldname]) || 0), 0);
    return out;
  });

  /** The selected rows on screen, in screen order (a Report field's frm.getSelectedRows). */
  export function selectedRows() { return grid.chosenOnScreen; }
  export function clearSelection() { grid.clearSelection(); }
  function exportRows(format: "csv" | "xlsx") {
    const { header, cells } = exportTable(grid.forExport, columns, getLinkTitle);
    // the totals are of every row on screen, so they only go along with all of them
    if (totals && !grid.chosen.length) cells.push(columns.map((c: any, i: number) => (totals[c.fieldname] !== undefined ? totals[c.fieldname] : i === 0 ? __("Total") : "")));
    downloadTable(format, filename, header, cells, sheetName);
  }
</script>

{#snippet cell(c: any, r: any)}
  {#if c.fieldtype === "Link" && r[c.fieldname]}{getLinkTitle(c.options, r[c.fieldname]) || r[c.fieldname]}
  {:else if (c.fieldname === "status" || isIndicatorSelect(c)) && r[c.fieldname]}<SelectIndicator value={r[c.fieldname]} field={c} iconOnly={c.optionIconOnly} />
  {:else}<span style:color={c.fieldtype === "Currency" && r[c.fieldname] < 0 ? "var(--red)" : undefined}>{formatValue(r[c.fieldname], c)}</span>{/if}
{/snippet}

<div class="card" style="overflow:auto">
  {#if exportable || filters.length || search.length || buttons.length || (selectable && grid.chosen.length)}
    <div class="grid-toolbar">
      {#if search.length}<GridSearch bind:value={grid.query} />{/if}
      {#if filters.length}<GridFilters {filters} active={grid.active} ontoggle={(i) => grid.toggleFilter(i)} />{/if}
      {#if selectable && grid.chosen.length}
        <span class="muted">{__("{0} selected", [grid.chosen.length])}</span>
        <GridActions {actions} rows={grid.chosenOnScreen} ondone={clearSelection} />
        <button class="btn sm" onclick={() => grid.toggleAll(false)}>{__("Clear selection")}</button>
      {/if}
      <span class="spacer"></span>
      <FieldButtons {buttons} small />
      {#if exportable}<GridExport onexport={exportRows} />{/if}
    </div>
  {/if}
  <table class="grid">
    <thead>
      <tr>
        {#if selectable}<th style="width:28px"><input type="checkbox" aria-label={__("Select all")} checked={grid.allSelected} onchange={(e) => grid.toggleAll(e.currentTarget.checked)} /></th>{/if}
        {#each columns as c}
          {@const sorted = grid.sort?.field === c.fieldname ? grid.sort!.order : null}
          <th class:num={num(c)} style="min-width:{c.width || 100}px" aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : sortable ? "none" : undefined}>
            {#if sortable}<button class="grid-sort" title={__("Sort by {0}", [c.label])} onclick={() => grid.toggleSort(c.fieldname)}>{c.label}{#if sorted} {sorted === "asc" ? "↑" : "↓"}{/if}</button>{:else}{c.label}{/if}
          </th>
        {/each}
      </tr>
    </thead>
    <tbody>
      {#each grid.viewRows as r, i}
        <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
        <tr class="row" class:row-click={!!onrowclick} onclick={onrowclick ? (e) => { if (isRowClick(e)) onrowclick(r); } : undefined}>
          {#if selectable}<td><input type="checkbox" aria-label={__("Select row {0}", [i + 1])} checked={grid.selected.has(r)} onchange={() => grid.toggle(r)} /></td>{/if}
          {#each columns as c}
            <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
            <td class:num={num(c)} onclick={forwardCellClick}>
              {#if clickable.has(c.fieldname) && r[c.fieldname] != null && r[c.fieldname] !== ""}
                <button type="button" class="cell-click" onclick={() => oncellclick?.(c.fieldname, r)}>{@render cell(c, r)}</button>
              {:else if c.fieldtype === "Link" && r[c.fieldname]}
                <a href={`${wsPrefix}/${seg(c.options)}/${encodeURIComponent(r[c.fieldname])}`} title={r[c.fieldname]}>{@render cell(c, r)}</a>
              {:else}{@render cell(c, r)}{/if}
            </td>
          {/each}
        </tr>
      {/each}
      {#if !grid.viewRows.length}<tr><td colspan={columns.length + (selectable ? 1 : 0) || 1} class="empty">{rows.length ? __("No rows match the filters") : __("No records")}</td></tr>{/if}
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
  .cell-click { all: unset; box-sizing: border-box; max-width: 100%; cursor: pointer; }
  .cell-click:hover { text-decoration: underline; }
  .cell-click:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; border-radius: 2px; }
  /* the click reaches the cell's button from anywhere in the cell (forwardCellClick) */
  td:has(> .cell-click) { cursor: pointer; }
  .totals td { font-weight: 600; border-top: 2px solid var(--border); }
</style>
