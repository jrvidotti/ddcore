<script module lang="ts">
  import type { FileInfo } from "$lib/api";
  // One lookup per file url for the page's lifetime: hovering again, or the
  // same file on another row, does not ask again.
  const infoCache = new Map<string, Promise<FileInfo | null>>();
</script>

<script lang="ts">
  import { api } from "$lib/api";
  import { showError } from "$lib/ui.svelte";
  import { __ } from "$lib/boot.svelte";
  import { formatDatetime } from "$lib/format";
  import Icon from "$lib/components/Icon.svelte";
  import { anchored } from "./floating";
  import { attachAction, fileLabel, formatFileSize, runAttachOperation, storedName } from "./attach-state";

  let { value, onchange, onbusychange = () => {}, readOnly = false, mandatory = false, doc = {}, fieldname = "", image = false, showFileName = false }: { value: any; onchange: (v: any) => void; onbusychange?: (busy: boolean) => void; readOnly?: boolean; mandatory?: boolean; doc?: any; fieldname?: string; image?: boolean; showFileName?: boolean } = $props();
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

  // The file's own name, size and type live on its File row; the url only
  // carries the random name it was stored under.
  let info = $state<FileInfo | null>(null);
  let hovering = $state(false);
  let anchorEl: HTMLElement | null = $state(null);
  function loadInfo(url: string) {
    if (!infoCache.has(url)) infoCache.set(url, api.fileInfo(url).catch(() => null));
    return infoCache.get(url)!;
  }
  $effect(() => {
    const url = value ? String(value) : "";
    info = null;
    if (!url || !(showFileName || hovering)) return;
    let live = true;
    loadInfo(url).then((i) => { if (live) info = i; });
    return () => { live = false; };
  });
  const label = $derived(fileLabel(info, value));
</script>

<div style="display:flex;gap:8px;align-items:center">
  {#if value}
    <a bind:this={anchorEl} href={value} target="_blank" rel="noopener" aria-label={label} class="preview"
       onmouseenter={() => (hovering = true)} onmouseleave={() => (hovering = false)}
       onfocus={() => (hovering = true)} onblur={() => (hovering = false)}>
      {#if image}
        <img class="thumb" src={value} alt={label} />
      {:else}
        <span class="icon"><Icon name="file" size={18} /></span>
      {/if}
    </a>
    {#if hovering}
      <div class="file-info" role="tooltip" use:anchored={{ anchor: anchorEl, content: info }}>
        <div class="name">{label}</div>
        {#if info}
          {#if formatFileSize(info.file_size)}<div><span class="muted">{__("Size")}:</span> {formatFileSize(info.file_size)}</div>{/if}
          {#if info.content_type}<div><span class="muted">{__("Type")}:</span> {info.content_type}</div>{/if}
          <div><span class="muted">{__("Uploaded")}:</span> {formatDatetime(info.creation)}{info.owner ? ` · ${info.owner}` : ""}</div>
        {/if}
      </div>
    {/if}
    {#if showFileName}
      <a href={value} target="_blank" rel="noopener" title={storedName(value)} style="font-size:13px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{label}</a>
    {/if}
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
  .preview { display: block; flex: none; }
  .thumb {
    width: 36px;
    height: 36px;
    object-fit: cover;
    border: 1px solid var(--border);
    border-radius: 4px;
    display: block;
  }
  .icon {
    width: 36px;
    height: 36px;
    display: flex;
    align-items: center;
    justify-content: center;
    border: 1px solid var(--border);
    border-radius: 4px;
    color: var(--muted);
  }
  .file-info {
    z-index: 50;
    max-width: 320px;
    padding: 6px 10px;
    font-size: 12px;
    line-height: 1.5;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgb(0 0 0 / 0.12);
    pointer-events: none;
  }
  .file-info .name { font-weight: 600; overflow-wrap: anywhere; }
</style>
