<script lang="ts">
  // Child table: inline editing by default, with an opt-in dialog-only mode.
  import type { Field, DocTypeMeta } from "$lib/meta";
  import { isLayout, isNumericFieldtype } from "$lib/meta";
  import { fitColumnWidth, inlineGridMinWidth, splitLastWord } from "$lib/grid-width";
  import Control from "./Control.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { evalExpr } from "$lib/expr";
  import { formatValue, isIndicatorSelect } from "$lib/format";
  import SelectIndicator from "$lib/components/SelectIndicator.svelte";
  import { __ } from "$lib/boot.svelte";
  import { confirm, dialog } from "$lib/ui.svelte";
  import type { FormController } from "$lib/form.svelte";
  import { applyRowChanges, changedKeys, confirmRowRemoval, createChildDraft, fileNameParts, gridEditMode, inlineEditable, overlayFields } from "./grid-state";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { declaredSort, downloadTable, exportBaseName, exportTable } from "$lib/grid-rows";
  import { GridRowsState } from "$lib/grid-rows-state.svelte";
  import FieldButtons from "./FieldButtons.svelte";
  import GridActions from "./GridActions.svelte";
  import GridExport from "./GridExport.svelte";
  import GridFilters from "./GridFilters.svelte";
  import GridSearch from "./GridSearch.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import { forwardCellClick, isRowClick } from "$lib/grid-click";

  let { frm, field, childMeta }: { frm: FormController; field: Field; childMeta: DocTypeMeta } = $props();
  const rows = $derived((frm.doc[field.fieldname!] ||= []) as any[]);
  const buttons = $derived(frm.fieldButtons[field.fieldname!] || []);
  const editable = $derived(frm.isFieldEditable(field));
  const cannotAdd = $derived(!!(field as any).cannotAddRows);
  const cannotDelete = $derived(!!(field as any).cannotDeleteRows);
  // frm.setDfProperty(table, "loading", true) while a script fetches the rows
  const loading = $derived(!!(field as any).loading);
  // the child's fields as this form shows them: a script may hide or relabel one (frm.setGridDfProperty)
  const childFields = $derived(overlayFields(childMeta.fields, frm.gridDfProps?.[field.fieldname!]));
  const columns = $derived(childFields.filter((f) => f.inListView && !isLayout(f) && !f.hidden));
  const totalCols = $derived(columns.reduce((s, c) => s + (c.columns || 2), 0));
  const editMode = $derived(gridEditMode(field));
  const dialogOnly = $derived(editMode === "dialog");
  // a column no cell can edit (a Signature) is edited in the row dialog
  const hasDialog = $derived(dialogOnly || childFields.some((f) => !f.inListView && !isLayout(f) && !f.hidden)
    || columns.some((c) => !inlineEditable(c.fieldtype)));

  // Sorting is display only: `rows` keeps its order (and each row its idx),
  // `viewRows` is what the table shows. Anything acting on a row resolves its
  // real position with indexOf, never with the position on screen.
  const presets = $derived(field.gridFilters || []);
  const searchFields = $derived(field.gridSearch || []);
  // preset filters and the search box (over its columns, hidden ones included) narrow what is shown
  const grid = new GridRowsState({ rows: () => rows, columns: () => childFields, presets: () => presets, search: () => searchFields, baseSort: () => declaredSort(field.gridSort) });
  const sortable = $derived(!!field.gridSortable);
  const selectable = $derived(!!field.gridSelect);
  const showIndex = $derived(field.gridIndex !== false);
  // gridLayout "fit": table-layout auto, each column as wide as its values, the leftover before the row buttons
  const fit = $derived(field.gridLayout === "fit");
  // an inline grid lays its columns out by `columns` (table-layout: fixed); below this width it scrolls sideways
  // instead of crushing the cells or spilling a number over the next column (grid-width.ts)
  const inlineMinWidth = $derived(inlineGridMinWidth(columns, (selectable ? 28 : 0) + (showIndex ? 44 : 0) + 72));
  const exportable = $derived(!!field.gridExport && !!frm.meta.permissions?.export);
  const canDelete = $derived(!dialogOnly && editable && !cannotDelete);

  $effect(() => frm.registerGridSelection(field.fieldname!, { rows: () => grid.chosenOnScreen, clear: () => grid.clearSelection() }));

  async function deleteSelected() {
    const n = grid.chosen.length;
    if (!n || !(await confirm(__("Delete {0} rows?", [n]), __("Delete selected"), { destructive: true }))) return;
    const idx = grid.chosen.map((r) => rows.indexOf(r)).filter((i) => i >= 0).sort((a, b) => b - a);
    for (const i of idx) frm.removeChild(field.fieldname!, i);
    grid.clearSelection();
    frm.trigger(field.fieldname!);
  }
  function exportRows(format: "csv" | "xlsx") {
    const { header, cells } = exportTable(grid.forExport, columns, getLinkTitle);
    downloadTable(format, exportBaseName(frm.doctype, frm.doc.id, field.fieldname), header, cells, field.label);
  }

  function editRow(row: any, isNew = false) {
    const i = isNew ? rows.length : rows.indexOf(row);
    const fields = childFields.filter((f) => !f.hidden).map((f) => ({ ...f, readOnly: f.readOnly || !editable || (!isNew && frm.isSetOnce(f, row, field.fieldname!)) }));
    const d = dialog({
      title: `${field.label} · ${isNew ? __("New") : `${__("Row")} ${i + 1}`}`, fields, values: { ...row }, size: "lg",
      primaryLabel: editable ? __("Apply") : __("Close"),
      primaryAction: (values, dlg) => {
        if (editable) {
          const changed = changedKeys(row, values);
          const target = isNew ? frm.addChild(field.fieldname!, values) : row;
          if (!isNew) applyRowChanges(target, values);
          frm.trigger(field.fieldname!, childMeta.name, target.id, target, changed);
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
  // a script acting on a click anywhere else on a row
  const rowClickable = $derived(frm.rowClickHandlers(field.fieldname!).length > 0);
  function clickRow(e: MouseEvent, row: any) { if (isRowClick(e)) frm.clickRow(field.fieldname!, row); }
  // an input has no width to fit: in a "fit" grid a column with inputs keeps room for one (grid-width.ts)
  const fitWidth = (c: Field) => fitColumnWidth(c, !dialogOnly && editable && !c.readOnly && inlineEditable(c.fieldtype));
  function rowEditable(f: Field, row: any) {
    return editable && !f.readOnly && !frm.isSetOnce(f, row, field.fieldname!) && !(f.readOnlyDependsOn && evalExpr(f.readOnlyDependsOn, row, frm.doc));
  }
</script>

<!-- a read-only cell's value: a Select that declares optionColors/optionIcons as its indicator, anything else as text -->
<!-- the required * and the help ⓘ follow the label's last word: inside the sort button, so a label that wraps
     keeps them on its line (a button is an inline-block, and a mark after it would drop to a line of its own) -->
{#snippet marks(c: Field, inButton: boolean)}
  {#if c.reqd}<span class="req" style="color:var(--red)" aria-hidden={inButton || undefined}>*</span>{/if}
  {#if c.description}<span class="help-tip" title={c.description} style="margin-left:4px;cursor:help;font-weight:normal;opacity:0.7" aria-hidden={inButton || undefined}>ⓘ</span>{/if}
{/snippet}

<!-- in a "fit" grid a label wraps between its words, the last one kept on a line with the sort arrow and the marks -->
{#snippet heading(c: Field, sorted: string | null, inButton: boolean)}
  {#if fit}
    {@const [head, last] = splitLastWord(c.label || "")}
    {head}<span style="white-space:nowrap">{last}{#if sorted} {sorted === "asc" ? "↑" : "↓"}{/if}{@render marks(c, inButton)}</span>
  {:else}{c.label}{#if sorted} {sorted === "asc" ? "↑" : "↓"}{/if}{@render marks(c, inButton)}{/if}
{/snippet}

{#snippet shown(c: Field, row: any, empty: string)}
  {#if isIndicatorSelect(c) && row[c.fieldname!]}<SelectIndicator value={row[c.fieldname!]} field={c} iconOnly={c.optionIconOnly} />{:else}{formatValue(row[c.fieldname!], c) || empty}{/if}
{/snippet}

<!-- a cell: in dialog mode a value (a file, a clickable value); inline, the control or the read-only value -->
{#snippet cell(c: Field, row: any)}
  {#if dialogOnly}
    {#if c.fieldtype === "Attach" && row[c.fieldname!]}
      {@const file = fileNameParts(row[c.fieldname!])}
      <a class="file-link" href={row[c.fieldname!]} target="_blank" rel="noopener" title={file.full} aria-label={__("Open file") + ": " + file.full}>
        <Icon name="paperclip" size={14} /><span class="file-stem">{file.stem}</span><span class="file-extension">{file.extension}</span>
      </a>
    {:else if clickable.has(c.fieldname!)}
      <button type="button" class="cell-click cell-value" title={formatValue(row[c.fieldname!], c)} onclick={() => frm.clickCell(field.fieldname!, c.fieldname!, row)}>{@render shown(c, row, "—")}</button>
    {:else}
      <span class="cell-value" title={formatValue(row[c.fieldname!], c)}>{@render shown(c, row, "—")}</span>
    {/if}
  {:else if !inlineEditable(c.fieldtype)}
    <button type="button" class="cell-click cell-value" title={editable ? __("Edit row") : __("View row")} onclick={() => editRow(row)}>{formatValue(row[c.fieldname!], c) || "—"}</button>
  {:else if rowEditable(c, row) && (!c.dependsOn || evalExpr(c.dependsOn, row, frm.doc))}
    <div data-own-click style="display:contents"><Control field={c} value={row[c.fieldname!]} onchange={(v) => { row[c.fieldname!] = v; frm.trigger(field.fieldname!, childMeta.name, row.id, row, [c.fieldname!]); }} doc={row} compact inGrid /></div>
  {:else if clickable.has(c.fieldname!)}
    <button type="button" class="cell-click" onclick={() => frm.clickCell(field.fieldname!, c.fieldname!, row)}>{@render shown(c, row, "")}</button>
  {:else}
    <span>{@render shown(c, row, "")}</span>
  {/if}
{/snippet}

<div class="field grid-field">
  <span class="label" class:sr-only={field.hideLabel}>{field.label}{#if frm.isFieldMandatory(field)}<span class="req">*</span>{/if}</span>
  <div class="card" style="overflow:auto">
    {#if exportable || presets.length || searchFields.length || buttons.length || (selectable && grid.chosen.length)}
      <div class="grid-toolbar">
        {#if searchFields.length}<GridSearch bind:value={grid.query} />{/if}
        {#if presets.length}<GridFilters filters={presets} active={grid.active} ontoggle={(i) => grid.toggleFilter(i)} />{/if}
        {#if selectable && grid.chosen.length}
          <span class="muted">{__("{0} selected", [grid.chosen.length])}</span>
          <GridActions actions={frm.gridActions[field.fieldname!] || []} rows={grid.chosenOnScreen} ondone={() => grid.clearSelection()} disabled={loading} />
          {#if canDelete}<button class="btn sm danger" disabled={loading} onclick={deleteSelected}><Icon name="trash" size={14} />{__("Delete selected")}</button>{/if}
          <button class="btn sm" onclick={() => grid.toggleAll(false)}>{__("Clear selection")}</button>
        {/if}
        <span class="spacer"></span>
        <FieldButtons {buttons} small disabled={loading} />
        {#if exportable}<GridExport onexport={exportRows} />{/if}
      </div>
    {/if}
    <table class="grid" class:fit-grid={fit} class:dialog-grid={dialogOnly && !fit} class:inline-grid={!dialogOnly && !fit} style={dialogOnly || fit ? undefined : `min-width:${inlineMinWidth}px`} class:loading={loading && grid.viewRows.length} aria-busy={loading || undefined}>
      <thead>
        <tr>
          {#if selectable}<th style="width:28px"><input type="checkbox" aria-label={__("Select all")} checked={grid.allSelected} onchange={(e) => grid.toggleAll(e.currentTarget.checked)} /></th>{/if}
          {#if showIndex}<th style="width:44px">#</th>{/if}
          {#each columns as c}
            {@const sorted = grid.sort?.field === c.fieldname ? grid.sort!.order : null}
            {@const width = fit ? (fitWidth(c) ? `${fitWidth(c)}px` : undefined) : `${(100 * (c.columns || 2)) / totalCols}%`}
            <th class:num={num(c)} style:width={width} title={c.description || ""} aria-sort={sorted === "asc" ? "ascending" : sorted === "desc" ? "descending" : sortable ? "none" : undefined}>
              {#if sortable}<button class="grid-sort" title={__("Sort by {0}", [c.label])} onclick={() => grid.toggleSort(c.fieldname!)}>{@render heading(c, sorted, true)}</button>{:else}{@render heading(c, null, false)}{/if}
            </th>
          {/each}
          <th class="row-buttons" style:width={fit ? "100%" : `${dialogOnly ? 48 : 72}px`}></th>
        </tr>
      </thead>
      <tbody>
        {#each grid.viewRows as row, i (row.id || row)}
          <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
          <tr class="row" class:row-click={rowClickable} onclick={rowClickable ? (e) => clickRow(e, row) : undefined}>
            {#if selectable}<td><input type="checkbox" aria-label={__("Select row {0}", [row.idx ?? i + 1])} checked={grid.selected.has(row)} onchange={() => grid.toggle(row)} /></td>{/if}
            {#if showIndex}<td class="muted">{row.idx ?? i + 1}</td>{/if}
            {#each columns as c}
              <!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_noninteractive_element_interactions -->
              {@const cap = fit && c.gridMaxWidth}
              <td class:num={num(c)} class:capped={cap} onclick={forwardCellClick}>
                {#if cap}<div class="fit-cap" style:max-width="{c.gridMaxWidth}px">{@render cell(c, row)}</div>{:else}{@render cell(c, row)}{/if}
              </td>
            {/each}
            <td style="white-space:nowrap;text-align:right" data-own-click>
              {#if hasDialog}<button class="btn sm icon" title={editable ? __("Edit row") : __("View row")} aria-label={editable ? __("Edit row") : __("View row")} onclick={() => editRow(row)}><Icon name={dialogOnly ? "pencil" : "chevron-right"} size={14} /></button>{/if}
              {#if canDelete}<button class="btn sm icon danger" title={__("Remove")} aria-label={__("Remove")} onclick={() => remove(row)}><Icon name="trash" size={14} /></button>{/if}
            </td>
          </tr>
        {/each}
        {#if !grid.viewRows.length}
          <tr><td colspan={columns.length + 1 + (selectable ? 1 : 0) + (showIndex ? 1 : 0)} class="muted grid-empty">
            {#if loading}<span class="grid-loading"><Spinner size={16} />{__("Loading...")}</span>{:else}{rows.length ? __("No rows match the filters") : __("No rows")}{/if}
          </td></tr>
        {/if}
      </tbody>
    </table>
    {#if editable && !cannotAdd}
      <div style="padding:8px"><button class="btn sm" disabled={loading} onclick={add}><Icon name="plus" size={14} />{__("Add row")}</button></div>
    {/if}
  </div>
</div>

<style>
  .grid-empty { text-align: center; padding: 14px; }
  .grid-loading { display: inline-flex; align-items: center; gap: 8px; }
  .grid-loading :global(.spinner-wrap) { padding: 0; }
  /* a reload: the old rows stay, dimmed and out of reach, until the new ones arrive */
  table.loading tbody { opacity: 0.45; pointer-events: none; transition: opacity 0.15s; }
  table.dialog-grid { table-layout: fixed; min-width: 720px; }
  /* gridLayout "fit": a column takes its widest value — a value does not wrap, a header wraps between its words — and
     the row buttons' column (width 100%) takes the rest, so the buttons stay at the right edge. A gridMaxWidth column
     is as wide as its value up to the cap (width: max-content, clamped by the max-width set inline), and wraps past it;
     a dialog-mode value (.cell-value) is cut with an ellipsis instead. */
  table.fit-grid { table-layout: auto; }
  table.fit-grid th:not(.row-buttons) { white-space: normal; }
  table.fit-grid td { white-space: nowrap; }
  table.fit-grid td.capped { white-space: normal; overflow-wrap: break-word; }
  .fit-cap { width: max-content; }
  table.dialog-grid td { overflow: hidden; }
  /* the widths come from `columns`: a long label wraps instead of taking the width from the other columns,
     and a read-only value with no break (a long code) wraps rather than spilling into the next cell */
  table.inline-grid { table-layout: fixed; }
  /* a word longer than its column ("Participação" in a `columns: 1`) is hyphenated in the desk's language
     (<html lang>), or broken where it must, instead of spilling into the next header */
  table.inline-grid th { white-space: normal; hyphens: auto; overflow-wrap: break-word; }
  table.inline-grid td:not(.num) { overflow-wrap: anywhere; }
  /* before .cell-value, which a dialog-mode cell button also carries */
  .cell-click { all: unset; box-sizing: border-box; max-width: 100%; cursor: pointer; }
  .cell-click:hover { text-decoration: underline; }
  .cell-click:focus-visible { outline: 2px solid var(--primary); outline-offset: 2px; border-radius: 2px; }
  /* the click reaches the cell's button from anywhere in the cell (forwardCellClick) */
  td:has(> .cell-click) { cursor: pointer; }
  .cell-value { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .file-link { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; white-space: nowrap; }
  .file-stem { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .file-extension { flex: none; }
</style>
