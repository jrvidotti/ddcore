<script lang="ts">
  import { api } from "$lib/api";
  import { showError } from "$lib/ui.svelte";
  import { __ } from "$lib/boot.svelte";
  import { attachAction, runAttachOperation } from "./attach-state";

  let { value, onchange, onbusychange = () => {}, readOnly = false, mandatory = false, doc = {}, fieldname = "" }: { value: any; onchange: (v: any) => void; onbusychange?: (busy: boolean) => void; readOnly?: boolean; mandatory?: boolean; doc?: any; fieldname?: string } = $props();
  let busy = $state(false);
  const action = $derived(attachAction(value, mandatory));
  function setBusy(value: boolean) { busy = value; onbusychange(value); }
  async function pick(e: Event) {
    const f = (e.target as HTMLInputElement).files?.[0];
    if (!f) return;
    try {
      await runAttachOperation(setBusy, async () => {
        const file = await api.upload(f, { doctype: doc?.parenttype || doc?.doctype, docname: doc?.parent || doc?.name, fieldname });
        onchange(file.file_url);
      });
    } catch (err) { showError(err); }
  }
</script>

<div style="display:flex;gap:8px;align-items:center">
  {#if value}
    <a href={value} target="_blank" rel="noopener" title={String(value).split("/").pop()} style="font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{String(value).split("/").pop()}</a>
    {#if !readOnly && action === "replace"}
      <label class="btn sm" style="cursor:pointer">{busy ? __("Uploading…") : __("Replace")}<input type="file" style="display:none" onchange={pick} disabled={busy} /></label>
    {:else if !readOnly && action === "remove"}
      <button class="btn sm" onclick={() => onchange(null)}>{__("Remove")}</button>
    {/if}
  {:else if !readOnly}
    <label class="btn sm" style="cursor:pointer">{busy ? __("Uploading…") : __("Attach")}<input type="file" style="display:none" onchange={pick} disabled={busy} /></label>
  {:else}
    <span class="muted small">—</span>
  {/if}
</div>
