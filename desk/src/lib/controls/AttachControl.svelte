<script lang="ts">
  import { api } from "$lib/api";
  import { showError } from "$lib/ui.svelte";
  import { __ } from "$lib/boot.svelte";
  import { attachAction, runAttachOperation } from "./attach-state";

  let { value, onchange, onbusychange = () => {}, readOnly = false, mandatory = false, doc = {}, fieldname = "", image = false }: { value: any; onchange: (v: any) => void; onbusychange?: (busy: boolean) => void; readOnly?: boolean; mandatory?: boolean; doc?: any; fieldname?: string; image?: boolean } = $props();
  // An Attach Image accepts only what a browser renders inline, which is the
  // same set the server enforces on upload and on save. SVG is not one of
  // them: it carries script.
  const accept = $derived(image ? "image/png,image/jpeg,image/gif,image/webp" : undefined);
  let busy = $state(false);
  const action = $derived(attachAction(value, mandatory));
  function setBusy(value: boolean) { busy = value; onbusychange(value); }
  async function pick(e: Event) {
    const f = (e.target as HTMLInputElement).files?.[0];
    if (!f) return;
    try {
      await runAttachOperation(setBusy, async () => {
        const file = await api.upload(f, { doctype: doc?.parenttype || doc?.doctype, docId: doc?.parent || doc?.id, fieldname });
        onchange(file.file_url);
      });
    } catch (err) { showError(err); }
  }
</script>

<div style="display:flex;gap:8px;align-items:center">
  {#if value && image}
    <a href={value} target="_blank" rel="noopener" title={String(value).split("/").pop()}>
      <img class="thumb" src={value} alt={String(value).split("/").pop()} />
    </a>
  {/if}
  {#if value}
    <a href={value} target="_blank" rel="noopener" title={String(value).split("/").pop()} style="font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{String(value).split("/").pop()}</a>
    {#if !readOnly && action === "replace"}
      <label class="btn sm" style="cursor:pointer">{busy ? __("Uploading…") : __("Replace")}<input type="file" style="display:none" {accept} onchange={pick} disabled={busy} /></label>
    {:else if !readOnly && action === "remove"}
      <button class="btn sm" onclick={() => onchange(null)}>{__("Remove")}</button>
    {/if}
  {:else if !readOnly}
    <label class="btn sm" style="cursor:pointer">{busy ? __("Uploading…") : image ? __("Choose image") : __("Attach")}<input type="file" style="display:none" {accept} onchange={pick} disabled={busy} /></label>
  {:else}
    <span class="muted small">—</span>
  {/if}
</div>

<style>
  .thumb {
    width: 36px;
    height: 36px;
    object-fit: cover;
    border: 1px solid var(--border);
    border-radius: 4px;
    display: block;
  }
</style>
