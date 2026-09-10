<script lang="ts">
  // Child table: inline editing by default, with an opt-in dialog-only mode.
  import type { Field, DocTypeMeta } from "$lib/meta";
  import { isLayout } from "$lib/meta";
  import Control from "./Control.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { evalExpr } from "$lib/expr";
  import { formatValue } from "$lib/format";
  import { __ } from "$lib/boot.svelte";
  import { confirm, dialog } from "$lib/ui.svelte";
  import type { FormController } from "$lib/form.svelte";
  import { applyRowChanges, confirmRowRemoval, createChildDraft, fileNameParts, gridEditMode } from "./grid-state";

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

  function editRow(row: any, i: number, isNew = false) {
    const fields = childMeta.fields.filter((f) => !f.hidden).map((f) => ({ ...f, readOnly: f.readOnly || !editable }));
    const d = dialog({
      title: `${field.label} · ${isNew ? __("New") : `${__("Row")} ${i + 1}`}`, fields, values: { ...row }, size: "lg",
      primaryLabel: editable ? __("Apply") : __("Close"),
      primaryAction: (values, dlg) => {
        if (editable) {
          const target = isNew ? frm.addChild(field.fieldname!, values) : row;
          if (!isNew) applyRowChanges(target, values);
          frm.trigger(field.fieldname!, childMeta.name, target.name);
        }
        dlg.hide();
      },
      ...(dialogOnly && !isNew && editable && !cannotDelete ? {
        dangerLabel: __("Delete attachment"),
        dangerAction: async (_values: Record<string, any>, dlg: any) => {
          await confirmRowRemoval(
            () => confirm(__("Delete this attachment?"), __("Delete attachment")),
            () => { remove(i); dlg.hide(); },
          );
        },
      } : {}),
    });
    d.show();
  }
  function add() {
    if (dialogOnly) {
      editRow(createChildDraft(field.fieldname!, childMeta.name, frm.doctype, rows.length), rows.length, true);
      return;
    }
    frm.addChild(field.fieldname!);
    frm.trigger(field.fieldname!);
  }
  function remove(i: number) { frm.removeChild(field.fieldname!, i); frm.trigger(field.fieldname!); }
  const num = (f: Field) => ["Int", "Float", "Currency", "Percent"].includes(f.fieldtype);
  function rowEditable(f: Field, row: any) {
    return editable && !f.readOnly && !(f.readOnlyDependsOn && evalExpr(f.readOnlyDependsOn, row, frm.doc));
  }
</script>

<div class="field grid-field">
  <span class="label">{field.label}{#if frm.isFieldMandatory(field)}<span class="req">*</span>{/if}</span>
  <div class="card" style="overflow:auto">
    <table class="grid" class:dialog-grid={dialogOnly}>
      <thead>
        <tr>
          <th style="width:44px">#</th>
          {#each columns as c}
            <th class:num={num(c)} style="width:{(100 * (c.columns || 2)) / totalCols}%" title={c.description || ""}>
              {c.label}
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
        {#each rows as row, i (row.name || i)}
          <tr class="row">
            <td class="muted">{i + 1}</td>
            {#each columns as c}
              <td class:num={num(c)}>
                {#if dialogOnly}
                  {#if c.fieldtype === "Attach" && row[c.fieldname!]}
                    {@const file = fileNameParts(row[c.fieldname!])}
                    <a class="file-link" href={row[c.fieldname!]} target="_blank" rel="noopener" title={file.full} aria-label={`${__("Abrir arquivo")}: ${file.full}`}>
                      <Icon name="paperclip" size={14} /><span class="file-stem">{file.stem}</span><span class="file-extension">{file.extension}</span>
                    </a>
                  {:else}
                    <span class="cell-value" title={formatValue(row[c.fieldname!], c)}>{formatValue(row[c.fieldname!], c) || "—"}</span>
                  {/if}
                {:else if rowEditable(c, row) && (!c.dependsOn || evalExpr(c.dependsOn, row, frm.doc))}
                  <Control field={c} value={row[c.fieldname!]} onchange={(v) => { row[c.fieldname!] = v; frm.trigger(field.fieldname!, childMeta.name, row.name); }} doc={row} compact inGrid />
                {:else}
                  <span>{formatValue(row[c.fieldname!], c)}</span>
                {/if}
              </td>
            {/each}
            <td style="white-space:nowrap;text-align:right">
              {#if hasDialog}<button class="btn sm icon" title={editable ? __("Edit row") : __("View row")} aria-label={editable ? __("Edit row") : __("View row")} onclick={() => editRow(row, i)}><Icon name={dialogOnly ? "pencil" : "chevron-right"} size={14} /></button>{/if}
              {#if !dialogOnly && editable && !cannotDelete}<button class="btn sm icon danger" title={__("Remove")} aria-label={__("Remove")} onclick={() => remove(i)}><Icon name="trash" size={14} /></button>{/if}
            </td>
          </tr>
        {/each}
        {#if !rows.length}
          <tr><td colspan={columns.length + 2} class="muted" style="text-align:center;padding:14px">{__("No rows")}</td></tr>
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
  .cell-value { display: block; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .file-link { display: flex; align-items: center; gap: 6px; min-width: 0; max-width: 100%; white-space: nowrap; }
  .file-stem { min-width: 0; overflow: hidden; text-overflow: ellipsis; }
  .file-extension { flex: none; }
</style>
