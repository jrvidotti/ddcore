<script lang="ts">
  // A Report field: a defineReport run for this document (its filters taken
  // from the document through `reportFilters`), shown as a read-only grid.
  import { untrack } from "svelte";
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import type { Field } from "$lib/meta";
  import type { FormController } from "$lib/form.svelte";
  import ReportGrid from "$lib/components/ReportGrid.svelte";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";
  import { exportBaseName, type GridSortState } from "$lib/grid-rows";
  import { reportFiltersFor } from "./report-field";

  let { frm, field }: { frm: FormController; field: Field } = $props();
  let result = $state<any>(null);
  let canExport = $state(false);
  let error = $state("");
  let loading = $state(false);
  const workspace = $derived(page.params.workspace || getRememberedWorkspace() || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");
  const baseSort = $derived<GridSortState | null>(field.gridSort?.field ? { field: field.gridSort.field, order: field.gridSort.order === "desc" ? "desc" : "asc" } : null);

  // runs on load, after a save or a reload (a new `modified`) and on refreshField
  $effect(() => {
    const id = frm.doc.id, modified = frm.doc.modified, bump = frm.fieldRefresh[field.fieldname!];
    if (frm.isNew) { result = null; return; }
    void bump;
    untrack(() => run(id, modified));
  });
  let seq = 0;
  async function run(_id: string, _modified: string) {
    const mine = ++seq;
    loading = true;
    try {
      const r = await api.report(field.options, reportFiltersFor(field, frm.doc));
      if (mine !== seq) return;
      result = r.result;
      canExport = r.meta?.canExport !== false;
      error = "";
    } catch (e: any) {
      if (mine === seq) { error = e?.message || String(e); result = null; }
    } finally { if (mine === seq) loading = false; }
  }
</script>

<div class="field grid-field">
  <span class="label">{field.label}</span>
  {#if frm.isNew}
    <div class="card muted" style="padding:14px;text-align:center">{__("Save the document first")}</div>
  {:else if error}
    <div class="card muted" style="padding:14px;text-align:center">{error}</div>
  {:else if result}
    <ReportGrid columns={result.columns || []} rows={result.rows || []} {wsPrefix}
      filename={exportBaseName(frm.doctype, frm.doc.id, field.fieldname)} sheetName={field.label}
      {baseSort} filters={field.gridFilters || []} sortable={!!field.gridSortable} selectable={!!field.gridSelect} exportable={!!field.gridExport && canExport} />
  {:else if loading}
    <div class="card muted" style="padding:14px;text-align:center">{__("Loading...")}</div>
  {/if}
</div>
