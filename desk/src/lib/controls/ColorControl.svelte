<script lang="ts">
  // A Color is stored as `#rrggbb`, lowercase. The field shows only the colour;
  // clicking it opens a popover with a grid of basic colours and, under
  // Advanced, a saturation/value square, a hue bar, the RGB channels and a
  // text box that makes a brand colour pasteable. The browser's own picker
  // cannot be drawn inside a page, so Advanced is this one.
  import { untrack } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { anchored } from "./floating";
  import { BASIC_COLORS, hexToRgb, hsvToRgb, normalizeColor, rgbToHex, rgbToHsv, type Hsv, type Rgb } from "./color-state";

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

  // The Advanced picker works in HSV so the hue survives dragging to grey or
  // black. It follows the stored value, and writes back when a drag ends.
  let hsv = $state<Hsv>({ h: 0, s: 0, v: 0 });
  const current = $derived(rgbToHex(hsvToRgb(hsv)));
  const rgb = $derived(hsvToRgb(hsv));
  $effect(() => {
    const rgbValue = hexToRgb(hex);
    // untracked: this runs when the stored value changes, not on every drag step
    if (rgbValue && hex !== untrack(() => current)) hsv = rgbToHsv(rgbValue);
  });

  const clamp01 = (n: number) => Math.min(1, Math.max(0, n));

  function setHsv(next: Hsv, save: boolean) {
    hsv = next;
    text = current;
    if (save && current !== hex) onchange(current);
  }

  // one handler for both the square and the hue bar: the pointer is captured
  // on press, so a drag that leaves the element still steers it
  function drag(e: PointerEvent, apply: (x: number, y: number) => Hsv) {
    if (e.button !== 0) return;
    const el = e.currentTarget as HTMLElement;
    el.setPointerCapture?.(e.pointerId);
    const move = (ev: PointerEvent) => {
      const r = el.getBoundingClientRect();
      setHsv(apply(clamp01((ev.clientX - r.left) / r.width), clamp01((ev.clientY - r.top) / r.height)), false);
    };
    const up = () => {
      el.removeEventListener("pointermove", move);
      el.removeEventListener("pointerup", up);
      el.removeEventListener("pointercancel", up);
      setHsv(hsv, true);
    };
    move(e);
    el.addEventListener("pointermove", move);
    el.addEventListener("pointerup", up);
    el.addEventListener("pointercancel", up);
  }

  function onSvKey(e: KeyboardEvent) {
    const step = e.shiftKey ? 0.1 : 0.01;
    const d: Record<string, [number, number]> = { ArrowLeft: [-step, 0], ArrowRight: [step, 0], ArrowUp: [0, step], ArrowDown: [0, -step] };
    if (!d[e.key]) return;
    e.preventDefault();
    setHsv({ ...hsv, s: clamp01(hsv.s + d[e.key][0]), v: clamp01(hsv.v + d[e.key][1]) }, true);
  }

  function onHueKey(e: KeyboardEvent) {
    const step = e.shiftKey ? 10 : 1;
    const d: Record<string, number> = { ArrowLeft: -step, ArrowDown: -step, ArrowRight: step, ArrowUp: step };
    if (!d[e.key]) return;
    e.preventDefault();
    setHsv({ ...hsv, h: Math.min(359, Math.max(0, hsv.h + d[e.key])) }, true);
  }

  function setChannel(k: keyof Rgb, v: string) {
    setHsv(rgbToHsv({ ...rgb, [k]: Math.min(255, Math.max(0, Math.round(Number(v) || 0))) }), true);
  }

  const canEyedrop = typeof window !== "undefined" && "EyeDropper" in window;
  async function eyedrop() {
    try {
      const { sRGBHex } = await new (window as any).EyeDropper().open();
      const picked = hexToRgb(sRGBHex);
      if (picked) setHsv(rgbToHsv(picked), true);
    } catch {
      // Escape during the pick rejects; nothing was chosen
    }
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
    <span class="swatch" class:no-colour={!hex} style:background-color={hex || null}></span>
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
          <div
            class="sv"
            style:background-color={`hsl(${hsv.h} 100% 50%)`}
            role="slider"
            tabindex="0"
            aria-label={__("Saturation and brightness")}
            aria-valuenow={Math.round(hsv.s * 100)}
            aria-valuetext={current}
            onpointerdown={(e) => drag(e, (x, y) => ({ ...hsv, s: x, v: 1 - y }))}
            onkeydown={onSvKey}
          >
            <span class="thumb" style:left={`${hsv.s * 100}%`} style:top={`${(1 - hsv.v) * 100}%`} style:background-color={current}></span>
          </div>
          <div class="row">
            {#if canEyedrop}
              <button type="button" class="btn icon sm eyedrop" aria-label={__("Pick a colour from the screen")} title={__("Pick a colour from the screen")} onclick={eyedrop}>
                <Icon name="pipette" size={15} />
              </button>
            {/if}
            <span class="preview" style:background-color={current}></span>
            <div
              class="hue"
              role="slider"
              tabindex="0"
              aria-label={__("Hue")}
              aria-valuemin={0}
              aria-valuemax={359}
              aria-valuenow={Math.round(hsv.h)}
              onpointerdown={(e) => drag(e, (x) => ({ ...hsv, h: Math.min(359, x * 360) }))}
              onkeydown={onHueKey}
            >
              <span class="thumb" style:left={`${(hsv.h / 360) * 100}%`} style:background-color={`hsl(${hsv.h} 100% 50%)`}></span>
            </div>
          </div>
          <div class="channels">
            {#each (["r", "g", "b"] as const) as k}
              <label>
                <input
                  type="number"
                  class="input"
                  min="0"
                  max="255"
                  value={rgb[k]}
                  onchange={(e) => setChannel(k, (e.target as HTMLInputElement).value)}
                />
                <span>{k.toUpperCase()}</span>
              </label>
            {/each}
            <label class="hex-label">
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
              <span>Hex</span>
            </label>
          </div>
        </div>
      {/if}

      {#if value}
        <div class="picker-foot">
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
  .swatch.no-colour {
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

  .advanced { display: flex; flex-direction: column; gap: 10px; }
  .sv {
    position: relative;
    height: 150px;
    border-radius: 6px;
    background-image: linear-gradient(to top, #000, transparent), linear-gradient(to right, #fff, transparent);
    cursor: crosshair;
    touch-action: none;
  }
  .thumb {
    position: absolute;
    width: 14px;
    height: 14px;
    border: 2px solid #fff;
    border-radius: 50%;
    box-shadow: 0 0 0 1px rgba(0, 0, 0, 0.35), 0 1px 3px rgba(0, 0, 0, 0.3);
    transform: translate(-50%, -50%);
    pointer-events: none;
  }
  .row { display: flex; align-items: center; gap: 8px; }
  .eyedrop { flex-shrink: 0; }
  .preview {
    flex-shrink: 0;
    width: 28px;
    height: 28px;
    border-radius: 50%;
    box-shadow: inset 0 0 0 1px rgba(0, 0, 0, 0.1);
  }
  .hue {
    position: relative;
    flex: 1;
    height: 12px;
    border-radius: 6px;
    background: linear-gradient(to right, #f00, #ff0, #0f0, #0ff, #00f, #f0f, #f00);
    cursor: pointer;
    touch-action: none;
  }
  .hue .thumb { top: 50%; }
  .sv:focus-visible, .hue:focus-visible { outline: 2px solid var(--primary, #2563eb); outline-offset: 2px; }
  .channels { display: grid; grid-template-columns: repeat(3, 1fr) 1.9fr; gap: 4px; }
  .channels label { display: flex; flex-direction: column; align-items: center; gap: 2px; font-size: 11px; color: var(--muted); }
  .channels .input { padding: 4px; text-align: center; min-height: 28px; font-variant-numeric: tabular-nums; }
  .channels input[type="number"] { -moz-appearance: textfield; appearance: textfield; }
  .channels input[type="number"]::-webkit-inner-spin-button,
  .channels input[type="number"]::-webkit-outer-spin-button { -webkit-appearance: none; margin: 0; }
  .hex {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    text-transform: lowercase;
  }

  .picker-foot { display: flex; justify-content: flex-end; margin-top: 8px; }
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
