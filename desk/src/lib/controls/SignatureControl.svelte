<script lang="ts">
  // Signature: a pad drawn with a finger, a pen or the mouse. Each stroke that
  // ends commits the pad, cropped to its strokes, as a PNG data URL — the
  // value Frappe stores too. A stored signature is shown as an image and never
  // drawn back onto the pad: "Sign again" starts from an empty one.
  import type { Field } from "$lib/meta";
  import { __ } from "$lib/boot.svelte";
  import { fitSize, isSignatureDataUrl, padBounds, trimBounds } from "./signature-state";

  let { field, value, onchange, readOnly = false, error = "", id = "" }:
    { field: Field; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string } = $props();

  const PAD_HEIGHT = 160;
  const shown = $derived(isSignatureDataUrl(value) ? value : "");

  // "pad" while the pad replaces a stored signature ("Sign again") and after
  // each stroke, so the pad stays up while the person keeps signing
  let mode = $state<"view" | "pad">("view");
  // "Sign again" can be taken back until the first stroke
  let canCancel = $state(false);
  // the value this control sent last: a different one arriving (a reload, a
  // form script) shows the image again
  let sent: string | null = null;
  $effect(() => {
    if ((value ?? null) !== sent) {
      mode = "view";
      canCancel = false;
    }
  });
  // a value that is not a signature cannot be shown, only replaced
  const padShown = $derived(!readOnly && (!shown || mode === "pad"));

  let canvas: HTMLCanvasElement | null = $state(null);
  let ctx: CanvasRenderingContext2D | null = null;
  let dpr = 1;
  let drawing = false;
  let last: { x: number; y: number } | null = null;

  // (re)size the pad to its box whenever it appears; resizing clears it,
  // which is right for a pad that starts empty
  $effect(() => {
    if (!canvas) return;
    dpr = Math.min(2, window.devicePixelRatio || 1);
    const w = canvas.clientWidth || 400;
    canvas.width = Math.round(w * dpr);
    canvas.height = Math.round(PAD_HEIGHT * dpr);
    ctx = canvas.getContext("2d", { willReadFrequently: true });
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.lineWidth = 2.2;
    ctx.lineCap = "round";
    ctx.lineJoin = "round";
    ctx.strokeStyle = "#111";
    ctx.fillStyle = "#111";
  });

  function point(e: PointerEvent) {
    const r = canvas!.getBoundingClientRect();
    return { x: e.clientX - r.left, y: e.clientY - r.top };
  }

  function down(e: PointerEvent) {
    if (!ctx || !canvas || readOnly) return;
    e.preventDefault();
    try { canvas.setPointerCapture(e.pointerId); } catch { /* a synthetic pointer */ }
    drawing = true;
    last = point(e);
    // a tap is a dot
    ctx.beginPath();
    ctx.arc(last.x, last.y, ctx.lineWidth / 2, 0, Math.PI * 2);
    ctx.fill();
  }

  function move(e: PointerEvent) {
    if (!drawing || !ctx || !last) return;
    e.preventDefault();
    const p = point(e);
    // a curve through the midpoints keeps a fast stroke from turning into
    // straight segments
    const mid = { x: (last.x + p.x) / 2, y: (last.y + p.y) / 2 };
    ctx.beginPath();
    ctx.moveTo(last.x, last.y);
    ctx.quadraticCurveTo(last.x, last.y, mid.x, mid.y);
    ctx.lineTo(p.x, p.y);
    ctx.stroke();
    last = p;
  }

  function up(e: PointerEvent) {
    if (!drawing) return;
    drawing = false;
    last = null;
    try { canvas?.releasePointerCapture(e.pointerId); } catch { /* already released */ }
    commit();
  }

  // Synchronous on purpose: the stroke's end is the edit, as a blur is for a
  // text box.
  function commit() {
    if (!ctx || !canvas) return;
    const b = trimBounds(ctx.getImageData(0, 0, canvas.width, canvas.height));
    if (!b) return;
    const crop = padBounds(b, Math.round(6 * dpr), canvas.width, canvas.height);
    const size = fitSize(crop.width, crop.height);
    const out = document.createElement("canvas");
    out.width = size.width;
    out.height = size.height;
    out.getContext("2d")?.drawImage(canvas, crop.x, crop.y, crop.width, crop.height, 0, 0, size.width, size.height);
    const url = out.toDataURL("image/png");
    if (!isSignatureDataUrl(url)) return;
    mode = "pad";
    canCancel = false;
    sent = url;
    onchange(url);
  }

  function clear() {
    if (ctx && canvas) {
      ctx.save();
      ctx.setTransform(1, 0, 0, 1, 0, 0);
      ctx.clearRect(0, 0, canvas.width, canvas.height);
      ctx.restore();
    }
    canCancel = false;
    sent = null;
    if (value !== null && value !== undefined) onchange(null);
  }

  function signAgain() {
    sent = value ?? null;
    mode = "pad";
    canCancel = true;
  }

  function cancel() {
    mode = "view";
    canCancel = false;
  }
</script>

<div class="signature" data-fieldname={field.fieldname} data-fieldtype="Signature">
  {#if padShown}
    <canvas bind:this={canvas} {id} class="signature-pad" class:error={!!error} style="height:{PAD_HEIGHT}px"
      aria-label={__("Signature pad")}
      onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up}></canvas>
    <div class="signature-actions">
      <button type="button" class="btn sm signature-clear" onclick={clear}>{__("Clear")}</button>
      {#if canCancel && shown}
        <button type="button" class="btn sm signature-cancel" onclick={cancel}>{__("Cancel")}</button>
      {/if}
    </div>
  {:else if shown}
    <div class="signature-view">
      <img class="signature-image" src={shown} alt={field.label || __("Signature")} />
    </div>
    {#if !readOnly}
      <div class="signature-actions">
        <button type="button" class="btn sm signature-again" onclick={signAgain}>{__("Sign again")}</button>
      </div>
    {/if}
  {:else}
    <div class="signature-empty muted">{value ? __("Invalid signature") : __("Not signed")}</div>
  {/if}
</div>

<style>
  /* the strokes are dark on any theme, so the pad and the image stay white */
  .signature-pad {
    display: block;
    width: 100%;
    background: #fff;
    border: 1px dashed var(--border, #ccc);
    border-radius: 6px;
    touch-action: none;
    cursor: crosshair;
  }
  .signature-pad.error { border-color: var(--red); }
  .signature-view {
    display: inline-block;
    max-width: 100%;
    padding: 6px;
    background: #fff;
    border: 1px solid var(--border, #ccc);
    border-radius: 6px;
  }
  .signature-image { display: block; max-width: 100%; max-height: 120px; }
  .signature-actions { display: flex; gap: 6px; margin-top: 6px; }
  .signature-empty { font-size: 13px; padding: 6px 0; }
</style>
