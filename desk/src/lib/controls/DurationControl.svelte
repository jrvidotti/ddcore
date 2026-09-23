<script lang="ts">
  // A Duration is whole seconds, edited in one box that reads `01d 02h 30m 00s`:
  // the arrows up and down step the unit under the caret, left and right move
  // between units, and digits fill the selected unit — as a native time input.
  import { __ } from "$lib/boot.svelte";
  import {
    durationSegments, durationUnits, joinDuration, parseDuration, segmentAt, splitDuration, stepDuration, typeDigit,
    type DurationLabels, type DurationOptions,
  } from "./duration-format";

  let { field, value, onchange, readOnly = false, error = "", id = "", inGrid = false }:
    { field: any; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string; inGrid?: boolean } = $props();

  const opts = $derived<DurationOptions>({
    hideDays: hasFlag("hideDays"),
    hideSeconds: hasFlag("hideSeconds"),
  });
  function hasFlag(flag: string): boolean {
    const o = field?.options;
    return Array.isArray(o) ? o.includes(flag) : o === flag;
  }

  const labels = $derived<DurationLabels>({ days: __("d"), hours: __("h"), minutes: __("m"), seconds: __("s") });
  const units = $derived(durationUnits(opts));
  const isSet = (v: any) => v !== null && v !== undefined && v !== "";
  const text = $derived(isSet(value) ? durationSegments(value, opts, labels).text : "");
  const placeholder = $derived(durationSegments(0, opts, labels).text);

  let seg = 0;
  // the digits typed so far on the selected unit
  let buffer = "";

  /** Writes `secs` into the box now, not on the next render, and selects unit `i`. */
  function show(el: HTMLInputElement, secs: any, i: number) {
    seg = Math.max(0, Math.min(units.length - 1, i));
    if (!isSet(secs)) {
      el.value = "";
      return;
    }
    const v = durationSegments(secs, opts, labels);
    el.value = v.text;
    el.setSelectionRange(v.segs[seg].start, v.segs[seg].end);
  }

  function commit(el: HTMLInputElement, secs: number | null, i = seg) {
    onchange(secs);
    show(el, secs, i);
  }

  function pickAtCaret(e: Event) {
    const el = e.currentTarget as HTMLInputElement;
    if (!isSet(value)) return;
    const { segs } = durationSegments(value, opts, labels);
    buffer = "";
    show(el, value, segmentAt(segs, el.selectionStart ?? 0));
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.altKey || e.ctrlKey || e.metaKey || e.key === "Tab" || e.key === "Enter" || e.key === "Escape") return;
    const el = e.currentTarget as HTMLInputElement;
    const unit = units[seg] ?? units[0];
    const k = e.key;
    if (k === "ArrowUp" || k === "ArrowDown") {
      e.preventDefault();
      buffer = "";
      commit(el, stepDuration(value, unit, (k === "ArrowUp" ? 1 : -1) * (e.shiftKey ? 10 : 1)));
    } else if (k === "ArrowLeft" || k === "ArrowRight" || k === "Home" || k === "End") {
      e.preventDefault();
      buffer = "";
      const to = k === "Home" ? 0 : k === "End" ? units.length - 1 : seg + (k === "ArrowLeft" ? -1 : 1);
      show(el, value, to);
    } else if (/^\d$/.test(k)) {
      e.preventDefault();
      const r = typeDigit(value, opts, unit, buffer, k);
      buffer = r.buffer;
      commit(el, r.secs, r.advance ? seg + 1 : seg);
    } else if (k === "Backspace" || k === "Delete") {
      e.preventDefault();
      buffer = "";
      // zeroes the unit; on a box that is already all zeroes, unsets the value
      const parts = splitDuration(Number(value) || 0, opts);
      commit(el, joinDuration(parts) ? joinDuration({ ...parts, [unit]: 0 }) : null);
    } else if (k === " " || labels[unit].startsWith(k.toLowerCase()) || unit[0] === k.toLowerCase()) {
      e.preventDefault();
      buffer = "";
      show(el, value, seg + 1);
    } else if (k.length === 1) {
      // nothing else belongs in the box
      e.preventDefault();
    }
  }

  function onPaste(e: ClipboardEvent) {
    e.preventDefault();
    const secs = parseDuration(e.clipboardData?.getData("text") ?? "", opts);
    if (secs !== null) commit(e.currentTarget as HTMLInputElement, secs);
  }

  // what a keyboard that sends no keys (a phone's, autofill) wrote instead
  function onInput(e: Event) {
    const el = e.currentTarget as HTMLInputElement;
    const secs = el.value.trim() ? parseDuration(el.value, opts) : null;
    if (secs !== null || !el.value.trim()) onchange(secs);
  }
</script>

<input
  {id}
  type="text"
  inputmode="numeric"
  class="input duration"
  class:grid={inGrid}
  class:error={!!error}
  readonly={readOnly}
  value={text}
  {placeholder}
  autocomplete="off"
  onfocus={readOnly ? undefined : (e) => { buffer = ""; show(e.currentTarget as HTMLInputElement, value, 0); }}
  onclick={readOnly ? undefined : pickAtCaret}
  onkeydown={readOnly ? undefined : onKeyDown}
  onpaste={readOnly ? undefined : onPaste}
  oninput={readOnly ? undefined : onInput}
  onblur={() => (buffer = "")}
/>

<style>
  .duration { font-variant-numeric: tabular-nums; }
  .duration:not(.grid) { max-width: 180px; }
</style>
