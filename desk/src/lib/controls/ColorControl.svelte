<script lang="ts">
  // A Color is stored as `#rrggbb`, lowercase. The field shows only the colour;
  // clicking it opens a popover with a grid of basic colours and, under
  // Advanced, the browser's picker next to a text box that makes a brand
  // colour pasteable.
  import { __ } from "$lib/boot.svelte";
  import { anchored } from "./floating";
  import { BASIC_COLORS, normalizeColor } from "./color-state";

  let { value, onchange, readOnly = false, error = "", id = "" }:
    { value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string } = $props();

  const hex = $derived(normalizeColor(value) ?? "");
  let text = $state("");
  $effect(() => { text = value ? String(value) : ""; });

  let open = $state(false);
  let tab = $state<"basic" | "advanced">("basic");
  let wrapEl: HTMLDivElement | null = $state(null);
  let triggerEl: HTMLButtonElement | null = $state(null);

  function toggle() {
    if (readOnly) return;
    tab = "basic";
    open = !open;
  }

  function close() {
    open = false;
    triggerEl?.focus();
  }

  function commit(v: string) {
    if (!v.trim()) { text = ""; onchange(null); return; }
    const n = normalizeColor(v);
    if (n) {
      text = n;
      onchange(n);
      return;
    }
    // what was typed is not a colour: show the value that is still stored,
    // rather than leaving text on screen that was never saved
    text = value ? String(value) : "";
  }

  function pick(c: string) {
    text = c;
    onchange(c);
    close();
  }

  function clear() {
    text = "";
    onchange(null);
    close();
  }

  $effect(() => {
    if (!open) return;
    function onDocClick(e: MouseEvent) {
      if (wrapEl && !wrapEl.contains(e.target as Node)) open = false;
    }
    window.addEventListener("mousedown", onDocClick);
    return () => window.removeEventListener("mousedown", onDocClick);
  });
</script>

<div class="color-wrap" bind:this={wrapEl}>
  <button
    bind:this={triggerEl}
    {id}
    type="button"
    class="input color-trigger"
    class:error={!!error}
    disabled={readOnly}
    aria-label={__("Colour")}
    aria-haspopup="dialog"
    aria-expanded={open}
    title={hex}
    onclick={toggle}
    onkeydown={(e) => { if (e.key === "Escape" && open) { e.stopPropagation(); open = false; } }}
  >
    <span class="swatch" class:empty={!hex} style:background-color={hex || null}></span>
  </button>

  {#if open && !readOnly}
    <div
      class="color-popover"
      use:anchored={{ anchor: wrapEl, content: tab }}
      role="dialog"
      aria-label={__("Colour")}
      tabindex="-1"
      onkeydown={(e) => { if (e.key === "Escape") { e.stopPropagation(); close(); } }}
    >
      <div class="tabs" role="tablist">
        <button type="button" role="tab" aria-selected={tab === "basic"} class:on={tab === "basic"} onclick={() => (tab = "basic")}>{__("Basic")}</button>
        <button type="button" role="tab" aria-selected={tab === "advanced"} class:on={tab === "advanced"} onclick={() => (tab = "advanced")}>{__("Advanced")}</button>
      </div>

      {#if tab === "basic"}
        <div class="grid">
          {#each BASIC_COLORS as c}
            <button
              type="button"
              class="cell"
              class:selected={c === hex}
              style:background-color={c}
              title={c}
              aria-label={c}
              onclick={() => pick(c)}
            ></button>
          {/each}
        </div>
      {:else}
        <div class="advanced">
          <input
            type="color"
            class="native"
            value={hex || "#000000"}
            aria-label={__("Colour")}
            onchange={(e) => commit((e.target as HTMLInputElement).value)}
          />
          <input
            type="text"
            class="input hex"
            value={text}
            placeholder="#rrggbb"
            spellcheck="false"
            oninput={(e) => (text = (e.target as HTMLInputElement).value)}
            onblur={(e) => commit((e.target as HTMLInputElement).value)}
            onkeydown={(e) => e.key === "Enter" && commit((e.currentTarget as HTMLInputElement).value)}
          />
        </div>
      {/if}

      {#if value}
        <div class="foot">
          <button type="button" class="btn-link" onclick={clear}>{__("Clear")}</button>
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .color-wrap { position: relative; width: 100%; }
  .color-trigger {
    display: flex;
    align-items: center;
    width: 100%;
    padding: 4px;
    cursor: pointer;
    text-align: left;
  }
  .color-trigger:disabled { cursor: default; }
  .swatch {
    display: block;
    width: 100%;
    min-height: 22px;
    border-radius: 4px;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.1);
  }
  /* no colour: a diagonal line across white, as colour pickers usually draw it */
  .swatch.empty {
    background: linear-gradient(to top right, transparent calc(50% - 1px), #d1d5db calc(50% - 1px), #d1d5db calc(50% + 1px), transparent calc(50% + 1px)), #fff;
  }

  .color-popover {
    position: fixed;
    z-index: 80;
    overflow: auto;
    background: #fff;
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.12);
    padding: 8px;
    width: 262px;
  }
  .tabs { display: flex; gap: 2px; margin-bottom: 8px; }
  .tabs button {
    border: 0;
    border-radius: 4px;
    padding: 3px 10px;
    background: transparent;
    font-size: 12px;
    color: var(--text-muted, #64748b);
    cursor: pointer;
  }
  .tabs button.on { background: var(--bg-hover, #e2e8f0); color: var(--text, #0f172a); }

  .grid { display: grid; grid-template-columns: repeat(10, 1fr); gap: 4px; }
  .cell {
    aspect-ratio: 1;
    padding: 0;
    border: 0;
    border-radius: 4px;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.1);
    cursor: pointer;
  }
  .cell:hover { transform: scale(1.12); }
  .cell.selected { outline: 2px solid var(--primary, #2563eb); outline-offset: 1px; }

  .advanced { display: flex; flex-direction: column; gap: 6px; }
  .native {
    width: 100%;
    height: 72px;
    padding: 2px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-input, #fff);
    cursor: pointer;
  }
  .hex {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    text-transform: lowercase;
  }

  .foot { display: flex; justify-content: flex-end; margin-top: 8px; }
  .btn-link {
    border: 0;
    background: transparent;
    padding: 2px 4px;
    font-size: 12px;
    color: var(--muted);
    cursor: pointer;
  }
  .btn-link:hover { color: var(--text); }
</style>
