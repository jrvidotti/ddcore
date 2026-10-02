<script lang="ts">
  // A dialog's File field: the chosen file goes to the script as
  // { name, size, type, base64 } and nowhere else. Nothing is uploaded, which
  // is what a credential needs; a file to keep is an Attach.
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { formatFileSize, runAttachOperation } from "./attach-state";
  import { fileMaxBytes, fileTooLarge, readDialogFile, type DialogFile } from "./file-state";

  let { value, onchange, onbusychange = () => {}, onerror = () => {}, readOnly = false, accept = undefined, maxBytes = undefined }: { value: DialogFile | null | undefined; onchange: (v: DialogFile | null) => void; onbusychange?: (busy: boolean) => void; onerror?: (message: string) => void; readOnly?: boolean; accept?: string; maxBytes?: number } = $props();
  let busy = $state(false);
  function setBusy(v: boolean) { busy = v; onbusychange(v); }
  async function pick(e: Event) {
    const input = e.target as HTMLInputElement;
    const f = input.files?.[0];
    // choosing the same file again has to fire change again
    input.value = "";
    if (!f) return;
    if (fileTooLarge(f.size, maxBytes)) {
      onerror(__("File is too large (limit {0})", [formatFileSize(fileMaxBytes(maxBytes))]));
      return;
    }
    onerror("");
    try {
      await runAttachOperation(setBusy, async () => onchange(await readDialogFile(f)));
    } catch (err) { showError(err); }
  }
  function remove() { onerror(""); onchange(null); }
</script>

<div style="display:flex;gap:8px;align-items:center">
  {#if value}
    <span class="icon"><Icon name="file" size={18} /></span>
    <span class="name" title={value.name}>{value.name}</span>
    <span class="muted small">{formatFileSize(value.size)}</span>
  {/if}
  {#if !readOnly}
    <label class="btn sm" style="cursor:pointer">{value ? __("Replace") : __("Choose file")}<input type="file" style="display:none" {accept} onchange={pick} disabled={busy} /></label>
    {#if value}<button class="btn sm" onclick={remove}>{__("Remove")}</button>{/if}
  {:else if !value}
    <span class="muted small">—</span>
  {/if}
</div>

<style>
  .icon { display: flex; flex: none; color: var(--muted); }
  .name { font-size: 13px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
</style>
