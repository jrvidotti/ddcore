<script lang="ts">
  // Records a short audio note from the microphone and hands it over as a
  // File (`value`, through `onchange`). Hidden where the browser cannot record
  // (no secure context, no getUserMedia or no MediaRecorder).
  import { onDestroy } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import {
    MAX_RECORDING_SECONDS, formatElapsed, isPermissionDenied, pickAudioMime,
    recordingAvailable, stopStream, voiceNoteName,
  } from "./audio-recorder-state";

  let { value = null, onchange, disabled = false }: { value?: File | null; onchange: (file: File | null) => void; disabled?: boolean } = $props();

  const available = recordingAvailable();
  let phase = $state<"idle" | "starting" | "recording">("idle");
  let elapsed = $state(0);
  let error = $state("");
  let previewUrl = $state("");

  let stream: MediaStream | null = null;
  let recorder: MediaRecorder | null = null;
  let chunks: Blob[] = [];
  let timer: ReturnType<typeof setInterval> | null = null;
  let startedAt = 0;
  let destroyed = false;

  // the preview plays the note the parent holds, so it survives a remount
  $effect(() => {
    const f = value;
    if (!f || typeof URL.createObjectURL !== "function") { previewUrl = ""; return; }
    const u = URL.createObjectURL(f);
    previewUrl = u;
    return () => URL.revokeObjectURL(u);
  });

  /** Ends the timer and turns the microphone off. */
  function release() {
    if (timer) clearInterval(timer);
    timer = null;
    stopStream(stream);
    stream = null;
  }

  async function start() {
    if (phase !== "idle") return;
    error = "";
    phase = "starting";
    try {
      stream = await navigator.mediaDevices.getUserMedia({ audio: true });
    } catch (e) {
      phase = "idle";
      error = isPermissionDenied(e)
        ? __("Microphone access was denied. Allow it in the browser to record a note.")
        : __("Could not start recording.");
      return;
    }
    if (destroyed) { release(); return; }
    const mime = pickAudioMime(MediaRecorder as any);
    let rec: MediaRecorder;
    try {
      rec = mime ? new MediaRecorder(stream, { mimeType: mime }) : new MediaRecorder(stream);
    } catch {
      release();
      phase = "idle";
      error = __("Could not start recording.");
      return;
    }
    recorder = rec;
    chunks = [];
    rec.ondataavailable = (ev) => { if (ev.data && ev.data.size > 0) chunks.push(ev.data); };
    rec.onstop = () => {
      release();
      recorder = null;
      if (destroyed) return;
      phase = "idle";
      const type = rec.mimeType || mime || "audio/webm";
      const blob = new Blob(chunks, { type });
      chunks = [];
      if (blob.size) onchange(new File([blob], voiceNoteName(type), { type: type.split(";")[0] }));
    };
    rec.start();
    startedAt = Date.now();
    elapsed = 0;
    phase = "recording";
    timer = setInterval(() => {
      elapsed = Math.floor((Date.now() - startedAt) / 1000);
      if (elapsed >= MAX_RECORDING_SECONDS) stop();
    }, 250);
  }

  function stop() {
    if (recorder && recorder.state !== "inactive") recorder.stop();
    else { release(); phase = "idle"; }
  }

  onDestroy(() => {
    destroyed = true;
    if (recorder && recorder.state !== "inactive") { try { recorder.stop(); } catch { /* already stopped */ } }
    release();
  });
</script>

{#if available}
  <div class="recorder">
    {#if value}
      {#if previewUrl}<audio controls src={previewUrl}></audio>{:else}<span class="small">{value.name}</span>{/if}
      <button type="button" class="btn sm" onclick={() => onchange(null)} {disabled}>
        <Icon name="trash" size={13} /> {__("Discard")}
      </button>
    {:else if phase === "recording"}
      <span class="rec-dot" aria-hidden="true"></span>
      <span class="elapsed">{formatElapsed(elapsed)} / {formatElapsed(MAX_RECORDING_SECONDS)}</span>
      <button type="button" class="btn sm" onclick={stop}>
        <Icon name="square" size={13} /> {__("Stop")}
      </button>
    {:else}
      <button type="button" class="btn sm" onclick={start} disabled={disabled || phase === "starting"}>
        <Icon name="mic" size={13} /> {__("Record an audio note")}
      </button>
    {/if}
  </div>
  {#if error}<div class="err" role="alert">{error}</div>{/if}
{/if}

<style>
  .recorder { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; min-height: 32px; }
  .recorder audio { height: 32px; max-width: 100%; flex: 1 1 200px; min-width: 0; }
  .elapsed { font-variant-numeric: tabular-nums; font-size: 13px; }
  .rec-dot { width: 10px; height: 10px; border-radius: 50%; background: var(--red, #dc2626); animation: pulse 1s ease-in-out infinite; }
  .err { font-size: 11px; color: var(--red); margin-top: 3px; }
  @keyframes pulse { 50% { opacity: .3; } }
</style>
