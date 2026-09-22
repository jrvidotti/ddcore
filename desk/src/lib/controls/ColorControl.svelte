<script lang="ts">
  // A Color is stored as `#rrggbb`, lowercase. The picker and the text box are
  // the same value; the text box is what makes a brand colour pasteable.
  import { __ } from "$lib/boot.svelte";
  import { normalizeColor } from "./color-state";

  let { value, onchange, readOnly = false, error = "", id = "" }:
    { value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string } = $props();

  const hex = $derived(normalizeColor(value) ?? "");
  let text = $state("");
  $effect(() => { text = value ? String(value) : ""; });

  function commit(v: string) {
    text = v;
    if (!v.trim()) { onchange(null); return; }
    const n = normalizeColor(v);
    if (n) onchange(n);
  }
</script>

<div class="color-row">
  <input
    type="color"
    class="swatch"
    disabled={readOnly}
    value={hex || "#000000"}
    aria-label={__("Colour")}
    onchange={(e) => commit((e.target as HTMLInputElement).value)}
  />
  <input
    {id}
    type="text"
    class="input hex"
    class:error={!!error}
    readonly={readOnly}
    value={text}
    placeholder="#rrggbb"
    spellcheck="false"
    oninput={(e) => (text = (e.target as HTMLInputElement).value)}
    onblur={(e) => commit((e.target as HTMLInputElement).value)}
    onkeydown={(e) => e.key === "Enter" && commit((e.currentTarget as HTMLInputElement).value)}
  />
  {#if value && !readOnly}
    <button type="button" class="btn sm" onclick={() => { text = ""; onchange(null); }}>{__("Clear")}</button>
  {/if}
</div>

<style>
  .color-row { display: flex; gap: 6px; align-items: center; }
  .swatch {
    width: 32px;
    height: 30px;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-input, #fff);
    cursor: pointer;
  }
  .hex { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; text-transform: lowercase; }
</style>
