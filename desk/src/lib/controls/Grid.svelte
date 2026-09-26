<script lang="ts">
  // Child table: inline editing by default, with an opt-in dialog-only mode.
  import type { Field, DocTypeMeta } from "$lib/meta";
  import { isLayout, isNumericFieldtype } from "$lib/meta";
  import Control from "./Control.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { evalExpr } from "$lib/expr";
  import { formatValue } from "$lib/format";
  import { __ } from "$lib/boot.svelte";
  import { confirm, dialog } from "$lib/ui.svelte";
  import type { FormController } from "$lib/form.svelte";
  import { applyRowChanges, confirmRowRemoval, createChildDraft, fileNameParts, gridEditMode } from "./grid-state";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { downloadTable, exportBaseName, exportTable, filterRows, nextSort, sortRows, type GridSortState } from "$lib/grid-rows";
  import GridExport from "./GridExport.svelte";
  import GridFilters from "./GridFilters.svelte";

  let { frm, field, childMeta }: { frm: FormController; field: Field; childMeta: DocTypeMeta } = $props();
  const rows = $derived((frm.doc[field.fieldname!] ||= []) as any[]);
  const editable = $derived(frm.isFieldEditable(field));
  const cannotAdd = $derived(!!(field as any).cannotAddRows);
  const cannotDelete = $derived(!!(field as any).cannotDeleteRows);
  const columns = $derived(childMeta.fields.filter((f) => f.inListView && !isLayout(f) && !f.hidden));
  const totalCols = $derived(columns.reduce((s, c) => s + (c.columns || 2), 0));
  const editMode = $derived(gridEditMode(field));
  const dialogOnly = $derived(editMode === "dialog");
  const hasDialog = $derived(dialogOnly || childMeta.fields.some((f) => !f.inListView && !isLayout(f) && !f.hidden));

  // Sorting is display only: `rows` keeps its order (and each row its idx),
  // `viewRows` is what the table shows. Anything acting on a row resolves its
  // real position with indexOf, never with the position on screen.
  const baseSort = $derived<GridSortState | null>(field.gridSort?.field ? { field: field.gridSort.field, order: field.gridSort.order === "desc" ? "desc" : "asc" } : null);
  let userSort = $state<GridSortState | null>(null);
  const sort = $derived(userSort ?? baseSort);
  // preset filters narrow what is shown the same way: selection, "select all"
  // and export follow the rows on screen
  const presets = $derived(field.gridFilters || []);
  let activeFilters = $state<Set<number> | null>(null);
  const active = $derived(activeFilters ?? new Set(presets.flatMap((f, i) => (f.default ? [i] : []))));
  const visibleRows = $derived(filterRows(rows, [...active].map((i) => presets[i]?.filters).filter(Boolean), childMeta.fields));
  const viewRows = $derived(sortRows(visibleRows, sort, childMeta.fields, getLinkTitle));
  const sortable = $derived(!!field.gridSortable);
  const selectable = $derived(!!field.gridSelect);
  const showIndex = $derived(field.gridIndex !== false);
  const exportable = $derived(!!field.gridExport && !!frm.meta.permissions?.export);
  let selected = $state<Set<any>>(new Set());
  // only rows still in the table count: a removed row, or every row after a
  // reload (which brings new row objects), drops out of the selection
  const chosen = $derived(visibleRows.filter((r) => selected.has(r)));
  const allSelected = $derived(visibleRows.length > 0 && chosen.length === visibleRows.length);
  const canDelete = $derived(!dialogOnly && editable && !cannotDelete);

  function toggleSort(c: Field) { userSort = nextSort(sort, c.fieldname!, baseSort); }
  function toggle(row: any) { const s = new Set(selected); if (s.has(row)) s.delete(row); else s.add(row); selected = s; }
  function toggleAll(on: boolean) { selected = on ? new Set(visibleRows) : new Set(); }
  function toggleFilter(i: number) { const s = new Set(active); if (s.has(i)) s.delete(i); else s.add(i); activeFilters = s; }
  async function deleteSelected() {
    const n = chosen.length;
    if (!n || !(await confirm(__("Delete {0} rows?", [n]), __("Delete selected"), { destructive: true }))) return;
    const idx = chosen.map((r) => rows.indexOf(r)).filter((i) => i >= 0).sort((a, b) => b - a);
    for (const i of idx) frm.removeChild(field.fieldname!, i);
    selected = new Set();
    frm.trigger(field.fieldname!);
  }
  function exportRows(format: "csv" | "xlsx") {
    const out = chosen.length ? viewRows.filter((r) => selected.has(r)) : viewRows;
    const { header, cells } = exportTable(out, columns, getLinkTitle);
    downloadTable(format, exportBaseName(frm.doctype, frm.doc.id, field.fieldname), header, cells, field.label);
  }

  function editRow(row: any, isNew = false) {
    const i = isNew ? rows.length : rows.indexOf(row);
    const fields = childMeta.fields.filter((f) => !f.hidden).map((f) => ({ ...f, readOnly: f.readOnly || !editable }));
    const d = dialog({
      title: `${field.label} · ${isNew ? __("New") : `${__("Row")} ${i + 1}`}`, fields, values: { ...row }, size: "lg",
      primaryLabel: editable ? __("Apply") : __("Close"),
      primaryAction: (values, dlg) => {
        if (editable) {
          const target = isNew ? frm.addChild(field.fieldname!, values) : row;
          if (!isNew) applyRowChanges(target, values);
          frm.trigger(field.fieldname!, childMeta.name, target.id, target);
        }
        dlg.hide();
      },
      ...(dialogOnly && !isNew && editable && !cannotDelete ? {
        dangerLabel: __("Delete attachment"),
        dangerAction: async (_values: Record<string, any>, dlg: any) => {
          await confirmRowRemoval(
            () => confirm(__("Delete this attachment?"), __("Delete attachment"), { destructive: true }),
            () => { remove(row); dlg.hide(); },
          );
        },
      } : {}),
    });
    d.show();
  }
  function add() {
    if (dialogOnly) {
      editRow(createChildDraft(field.fieldname!, childMeta.name, frm.doctype, rows.length), true);
      return;
    }
    frm.addChild(field.fieldname!);
    frm.trigger(field.fieldname!);
  }
  function remove(row: any) {
    const i = rows.indexOf(row);
    if (i < 0) return;
    frm.removeChild(field.fieldname!, i);
    frm.trigger(field.fieldname!);
  }
  const num = (f: Field) => isNumericFieldtype(f.fieldtype);
  // columns a form script acts on when their (read-only) cell is clicked
  const clickable = $derived(new Set(columns.filter((c) => frm.cellClickHandlers(field.fieldname!, c.fieldname!).length).map((c) => c.fieldname!)));
  function rowEditable(f: Field, row: any) {
    return editable && !f.readOnly && !(f.readOnlyDependsOn && evalExpr(f.readOnlyDependsOn, row, frm.doc));
  }
</script>

<div class="field grid-field">
  <span class="label">{field.label}{#if frm.isFieldMandatory(field)}<span class="req">*</span>{/if}</span>
  <div class="card" style="overflow:auto">
    {#if exportable || presets.length || (selectable && chosen.length)}
      <div class="grid-toolbar">
        {#if presets.length}<GridFilters filters={presets} {active} ontoggle={toggleFilter} />{/if}
        {#if selectable && chosen.length}
          <span class="muted">{__("{0} selected", [chosen.length])}</span>
          {#if canDelete}<button class="btn sm danger" onclick={deleteSelected}><Icon name="trash" size={14} />{__("Delete selected")}</button>{/if}
          <button class="btn sm" onclick={() => toggleAll(false)}>{__("Clear selection")}</button>
        {/if}
        <span class="spacer"></span>
        {#if exportable}<GridExport onexport={exportRows} />{/if}
      </div>
    {/if}
    <table class="grid" class:dialog-grid={dialogOnly}>
      <thead>
        <tr>
          {#if selectable}<th style="width:28px"><input type="checkbox" aria-label={__("Select all")} checked={allSelected} onchange={(e) => toggleAll(e.currentTarget.checked)} /></th>{/if}
          {#if showIndex}<th style="width:44px">#</th>{/if}
          {#each columns as c}
            {@const sorted = sort?.field === c.fieldname ? sort!.order : null}
            <th class:num={num(c)} style="width:{(100 * (c.columns || 2)) / totalCols}%" title={c.description || ""} aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : sortable ? "none" : undefined}>
              {#if sortable}<button class="grid-sort" title={__("Sort by {0}", [c.label])} onclick={() => toggleSort(c)}>{c.label}{#if sorted} {sorted === "asc" ? "↑" : "↓"}{/if}</button>{:else}{c.label}{/if}
              {#if c.reqd}<span class="req" style="color:var(--red)">*</span>{/if}
              {#if c.description}
                <span class="help-tip" title={c.description} style="margin-left:4px;cursor:help;font-weight:normal;opacity:0.7">ⓘ</span>
              {/if}
            </th>
          {/each}
          <th style="width:{dialogOnly ? 48 : 72}px"></th>
        </tr>
      </thead>
      <tbody>
        {#each viewRows as row, i (row.id || row)}
          <tr class="row">
            {#if selectable}<td><input type="checkbox" aria-label={__("Select row {0}", [row.idx ?? i + 1])} checked={selected.has(row)} onchange={() => toggle(row)} /></td>{/if}
            {#if showIndex}<td class="muted">{row.idx ?? i + 1}</td>{/if}
            {#each columns as c}
              <td class:num={num(c)}>
                {#if dialogOnly}
                  {#if c.fieldtype === "Attach" && row[c.fieldname!]}
                    {@const file = fileNameParts(row[c.fieldname!])}
                    <a class="file-link" href={row[c.fieldname!]} target="_blank" rel="noopener" title={file.full} aria-label={__("Open file") + ": " + file.full}>
                      <Icon name="paperclip" size={14} /><span class="file-stem">{file.stem}</span><span class="file-extension">{file.extension}</span>
                    </a>
                  {:else if clickable.has(c.fieldname!)}
                    <button type="button" class="cell-click cell-value" title={formatValue(row[c.fieldname!], c)} onclick={() => frm.clickCell(field.fieldname!, c.fieldname!, row)}>{formatValue(row[c.fieldname!], c) || "—"}</button>
                  {:else}
                    <span class="cell-value" title={formatValue(row[c.fieldname!], c)}>{formatValue(row[c.fieldname!], c) || "—"}</span>
                  {/if}
                {:else if rowEditable(c, row) && (!c.dependsOn || evalExpr(c.dependsOn, row, frm.doc))}
                  <Control field={c} value={row[c.fieldname!]} onchange={(v) => { row[c.fieldname!] = v; frm.trigger(field.fieldname!, childMeta.name, row.id, row); }} doc={row} compact inGrid />
                {:else if clickable.has(c.fieldname!)}
                  <button type="button" class="cell-click" onclick={() => frm.clickCell(field.fieldname!, c.fieldname!, row)}>{formatValue(row[c.fieldname!], c)}</button>
                {:else}
                  <span>{formatValue(row[c.fieldname!], c)}</span>
                {/if}
              </td>
            {/each}
            <td style="white-space:nowrap;text-align:right">
              {#if hasDialog}<button class="btn sm icon" title={editable ? __("Edit row") : __("View row")} aria-label={editable ? __("Edit row") : __("View row")} onclick={() => editRow(row)}><Icon name={dialogOnly ? "pencil" : "chevron-right"} size={14} /></button>{/if}
              {#if canDelete}<button class="btn sm icon danger" title={__("Remove")} aria-label={__("Remove")} onclick={() => remove(row)}><Icon name="trash" size={14} /></button>{/if}
            </td>
          </tr>
        {/each}
        {#if !viewRows.length}
          <tr><td colspan={columns.length + 1 + (selectable ? 1 : 0) + (showIndex ? 1 : 0)} class="muted" style="text-align:center;padding:14px">{rows.length ? __("No rows match the filters") : __("No rows")}</td></tr>
        {/if}
      </tbody>
    </table>
    {#if editable && !cannotAdd}
      <div style="padding:8px"><button class="btn sm" onclick={add}><Icon name="plus" size={14} />{__("Add row")}</button></div>
    {/if}
  </div>
</div>

<style>
  table.dialog-grid { table-layout: fixed; min-width: 720px; }
  table.dialog-grid td { overflow: hidden; }
  /* before .cell-value, which a dialog-mode cell button also carries */
  .cell-click { all: unset; box-sizing: border-box; max-width: 100%; cursor: pointer; }
  .cell-click:hover { text-decoration: underline; }
  .cell-click:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; border-radius: 2px; }
  .cell-value { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .file-link { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; white-space: nowrap; }
  .file-stem { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .file-extension { flex: none; }
</style>
