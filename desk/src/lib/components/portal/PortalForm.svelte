<script lang="ts">
  // A document as a portal page shows it (OPS-10): only the page's fields,
  // and only its editable ones open for typing. The server holds the same
  // lists and refuses anything outside them; this keeps the screen honest.
  import Control from "$lib/controls/Control.svelte";
  import { cellWidthClass, LINE_SLOTS, packLines } from "$lib/components/form-layout";
  import { registerTitles } from "$lib/titles.svelte";
  import { statusColor } from "$lib/format";
  import { showError, toast } from "$lib/ui.svelte";
  import { __ } from "$lib/boot.svelte";
  import { editableValues, portalApi, type PortalPageMeta } from "$lib/portal";

  let { meta, portal, doc: initial, isNew = false, onsaved = () => {} }: {
    meta: PortalPageMeta; portal: string; doc: Record<string, any>; isNew?: boolean;
    onsaved?: (doc: Record<string, any>) => void;
  } = $props();

  let doc = $state<Record<string, any>>({});
  let busy = $state(false);
  let uploading = $state(0);
  $effect(() => {
    // the controls read `doctype` (and `id`) to address their uploads
    doc = { ...initial, doctype: meta.page.doctype };
    if (initial?._linkTitles) registerTitles(initial._linkTitles);
  });

  const canEdit = $derived(isNew ? meta.page.create : meta.page.write);
  const editable = $derived(new Set(meta.page.editable || []));
  const wfState = $derived(meta.page.stateField ? doc[meta.page.stateField] : "");
  const lines = $derived(packLines(meta.fields));

  function readOnly(fieldname?: string) {
    return !canEdit || !fieldname || !editable.has(fieldname);
  }

  async function save(e: Event) {
    e.preventDefault();
    busy = true;
    try {
      const values = editableValues(doc, meta.page.editable);
      const saved = isNew
        ? await portalApi.create(portal, meta.page.name, values)
        : await portalApi.update(portal, meta.page.name, doc.id, { ...values, modified: doc.modified });
      if (saved?._linkTitles) registerTitles(saved._linkTitles);
      toast(__("Saved"), { indicator: "green" });
      onsaved(saved);
    } catch (err) { showError(err); } finally { busy = false; }
  }
</script>

<form class="form-section portal-form" onsubmit={save}>
  {#if wfState}
    <div class="state"><span class="indicator {statusColor(wfState)}">{__(wfState)}</span></div>
  {/if}
  {#each lines as line}
    <div class="form-row">
      {#each line as cell}
        {@const f = cell.field}
        <div class="form-cell {cellWidthClass(cell.slots, LINE_SLOTS)}">
          {#if f}
            <Control field={f} value={doc[f.fieldname || ""]} {doc}
              readOnly={readOnly(f.fieldname)}
              onchange={(v) => (doc[f.fieldname || ""] = v)}
              onbusychange={(b) => (uploading += b ? 1 : -1)}
              linkSearch={(txt) => portalApi.search(portal, meta.page.name, f.fieldname || "", txt)} />
          {/if}
        </div>
      {/each}
    </div>
  {/each}
  {#if canEdit}
    <div class="actions">
      <button class="btn primary" disabled={busy || uploading > 0}>{isNew ? __("Submit") : __("Save")}</button>
    </div>
  {/if}
</form>

<style>
  .portal-form { max-width: 880px; }
  .state { margin-bottom: 12px; }
  .actions { display: flex; justify-content: flex-end; margin-top: 8px; }
</style>
