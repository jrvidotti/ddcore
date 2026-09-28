<script lang="ts">
  // Barcode: a text box, the code it draws underneath, and — where the
  // browser can read one — a camera button that fills the box. The image comes
  // from /api/barcode, the encoder print uses too, so what the form previews is
  // what the label prints; a value the server refuses shows a message instead,
  // and the save gives the reason.
  import type { Field } from "$lib/meta";
  import { __ } from "$lib/boot.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { barcodeUrl, committedBarcode, detectorFormat, scanAvailable, symbologyOf } from "./barcode-state";

  let { field, value, onchange, readOnly = false, error = "", id = "", preview = true }:
    { field: Field; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string;
      /** false in list filters and grid cells, where only the text fits */
      preview?: boolean } = $props();

  const symbology = $derived(symbologyOf(field));
  const canScan = scanAvailable();

  let text = $state("");
  let focused = $state(false);
  $effect(() => {
    if (!focused) text = value === null || value === undefined ? "" : String(value);
  });

  // The preview follows the typing, a moment behind it, so each keystroke
  // does not fetch an image.
  let shown = $state("");
  $effect(() => {
    const next = barcodeUrl(symbology, text);
    const t = setTimeout(() => (shown = next), 300);
    return () => clearTimeout(t);
  });
  let broken = $state(false);
  $effect(() => {
    void shown;
    broken = false;
  });

  function commit(t: string) {
    const v = committedBarcode(t);
    text = v ?? "";
    if (v !== (value ?? null)) onchange(v);
  }

  // Synchronous on purpose: the save shortcut blurs the input and reads the
  // value right after.
  function onblur() {
    focused = false;
    if (!readOnly) commit(text);
  }

  function onkeydown(e: KeyboardEvent) {
    // a USB scanner types the code and presses Enter
    if (e.key === "Enter" && !readOnly) {
      commit(text);
      e.preventDefault();
    }
  }

  // ---- camera
  let scanning = $state(false);
  let scanError = $state("");
  let video: HTMLVideoElement | null = $state(null);
  let overlay: HTMLDivElement | null = $state(null);
  // the overlay takes the focus so Escape reaches it
  $effect(() => {
    if (scanning) overlay?.focus();
  });
  let stream: MediaStream | null = null;
  let timer: ReturnType<typeof setTimeout> | undefined;

  function stop() {
    clearTimeout(timer);
    timer = undefined;
    stream?.getTracks().forEach((t) => t.stop());
    stream = null;
    scanning = false;
  }

  async function scan() {
    scanError = "";
    scanning = true;
    try {
      stream = await navigator.mediaDevices.getUserMedia({ video: { facingMode: "environment" }, audio: false });
    } catch {
      scanError = __("The camera is not available");
      return;
    }
    if (!scanning) return stop(); // closed while the browser asked for permission
    const Detector = (window as any).BarcodeDetector;
    const detector = new Detector({ formats: [detectorFormat(symbology)] });
    if (video) {
      video.srcObject = stream;
      await video.play().catch(() => {});
    }
    const tick = async () => {
      if (!scanning || !video) return;
      try {
        const found = await detector.detect(video);
        const raw = found?.[0]?.rawValue;
        if (raw) {
          stop();
          commit(String(raw));
          return;
        }
      } catch {
        // a frame that is not ready yet; the next one will be
      }
      if (scanning) timer = setTimeout(tick, 200);
    };
    tick();
  }

  // leaving the form with the camera open must not leave it on
  $effect(() => () => stop());

  function overlayKey(e: KeyboardEvent) {
    if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      stop();
    }
  }
</script>

<div class="barcode">
  <div class="barcode-row">
    <input {id} class="input" class:error={!!error} type="text" readonly={readOnly} value={text} autocomplete="off"
      maxlength={field.length || undefined} data-fieldname={field.fieldname} data-fieldtype="Barcode"
      onfocus={() => (focused = true)} oninput={(e) => (text = (e.target as HTMLInputElement).value)} {onblur} {onkeydown} />
    {#if canScan && !readOnly && preview}
      <button type="button" class="btn icon scan-btn" title={__("Scan")} aria-label={__("Scan")} onclick={scan}>
        <Icon name="scan-line" size={16} />
      </button>
    {/if}
  </div>
  {#if preview && shown}
    {#if broken}
      <div class="barcode-invalid muted">{__("Invalid barcode")}</div>
    {:else}
      <img class="barcode-preview" class:qr={symbology === "QR"} src={shown} alt={text} onerror={() => (broken = true)} onload={() => (broken = false)} />
    {/if}
  {/if}
</div>

{#if scanning}
  <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
  <div bind:this={overlay} class="scan-overlay" role="dialog" aria-modal="true" aria-label={__("Scan")} tabindex="-1" onkeydown={overlayKey}>
    <div class="scan-box">
      {#if scanError}
        <div class="scan-msg">{scanError}</div>
      {:else}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video bind:this={video} playsinline muted></video>
        <div class="scan-msg muted">{__("Point the camera at the barcode")}</div>
      {/if}
      <button type="button" class="btn" onclick={stop}>{__("Close")}</button>
    </div>
  </div>
{/if}

<style>
  .barcode-row { display: flex; gap: 6px; align-items: flex-start; }
  .barcode-row .input { flex: 1 1 auto; min-width: 0; }
  .scan-btn { min-height: 34px; }
  .barcode-preview {
    display: block;
    margin-top: 6px;
    height: 56px;
    max-width: 100%;
    background: #fff;
  }
  .barcode-preview.qr { height: 96px; }
  .barcode-invalid { margin-top: 6px; font-size: 12px; }
  .scan-overlay {
    position: fixed;
    inset: 0;
    z-index: 1000;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.6);
    padding: 16px;
  }
  .scan-box {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
    padding: 12px;
    border-radius: 8px;
    background: var(--surface, #fff);
    box-shadow: var(--shadow);
    max-width: min(480px, 100%);
  }
  .scan-box video { width: 100%; max-height: 60vh; border-radius: 4px; background: #000; }
  .scan-msg { font-size: 13px; text-align: center; }
</style>
