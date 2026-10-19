<script lang="ts" module>
  import type { DateStep } from "./date-format.ts";

  export interface PickerFormat {
    format: (v: any) => string;
    parse: (text: string) => { iso: string } | null;
    mask: (raw: string) => string;
    step: (text: string, caret: number, delta: number, todayIso: string) => DateStep | null;
    /** Digits in a complete entry: 8 for a date, 6 for a month. */
    digits: number;
  }
</script>

<!-- The masked text input and calendar popover that the Date and Month controls share. -->
<script lang="ts">
  import type { Snippet } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import { anchored } from "./floating";
  import type { Field } from "$lib/meta";
  import Icon from "$lib/components/Icon.svelte";
  import { dateShape } from "$lib/locale";

  let {
    field,
    fieldtype,
    value,
    onchange,
    readOnly = false,
    error = "",
    id = "",
    fmt,
    placeholder,
    todayIso,
    todayValue,
    todayLabel,
    openLabel,
    compact = false,
    nav,
    onview,
    grid,
  }: {
    field: Field;
    fieldtype: string;
    value: any;
    onchange: (v: any) => void;
    readOnly?: boolean;
    error?: string;
    id?: string;
    fmt: PickerFormat;
    placeholder: string;
    todayIso: string;
    /** What the popover's "today" button picks. */
    todayValue: string;
    todayLabel: string;
    openLabel: string;
    compact?: boolean;
    nav: { title: string; prevLabel: string; nextLabel: string; onprev: () => void; onnext: () => void };
    /** The popover should show the period of this ISO date. */
    onview: (iso: string) => void;
    grid: Snippet<[(iso: string) => void]>;
  } = $props();

  let text = $state("");
  let focused = $state(false);
  let open = $state(false);
  let wrapEl: HTMLDivElement | null = $state(null);
  let inputEl: HTMLInputElement | null = $state(null);

  $effect(() => {
    if (!focused) {
      text = fmt.format(value);
    }
  });

  function togglePicker() {
    if (readOnly) return;
    if (!open) {
      onview(fmt.parse(value)?.iso ?? todayIso);
      open = true;
    } else {
      open = false;
    }
  }

  function onInput(e: Event) {
    const target = e.target as HTMLInputElement;
    text = fmt.mask(target.value);
    // written back by hand: when the mask drops a keystroke, `text` does not
    // change, so the render would leave the extra character in the input
    target.value = text;
    // complete once every digit is in, whatever the separator's width
    if (text.replace(/\D/g, "").length === fmt.digits) {
      const p = fmt.parse(text);
      if (p) {
        onview(p.iso);
        onchange(p.iso);
      }
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.key === "Backspace") {
      const target = e.target as HTMLInputElement;
      const { sep } = dateShape();
      if (
        text.endsWith(sep) &&
        target.selectionStart === target.selectionEnd &&
        target.selectionStart === text.length
      ) {
        // the separator was typed by the mask, so it goes with the digit before it
        e.preventDefault();
        text = text.slice(0, -(sep.length + 1));
      }
    } else if (e.key === "Enter") {
      commit();
      (e.target as HTMLInputElement).blur();
      open = false;
    } else if (e.key === "Escape") {
      if (open) {
        open = false;
        e.stopPropagation();
      }
    } else if ((e.key === "ArrowDown" || e.key === "Down") && e.altKey) {
      e.preventDefault();
      togglePicker();
    } else if ((e.key === "ArrowUp" || e.key === "ArrowDown") && !e.altKey && !e.ctrlKey && !e.metaKey) {
      // the part under the caret steps, as in a native date input
      if (readOnly) return;
      e.preventDefault();
      const target = e.target as HTMLInputElement;
      const s = fmt.step(text, target.selectionStart ?? 0, e.key === "ArrowUp" ? 1 : -1, todayIso);
      if (s) {
        text = s.text;
        onview(s.iso);
        onchange(s.iso);
        // written now, not on the next render, so the selection has text to land on
        target.value = s.text;
        target.setSelectionRange(s.start, s.end);
      }
    }
  }

  function commit() {
    if (!text) {
      if (value) onchange(null);
      return;
    }
    const p = fmt.parse(text);
    if (p) {
      onchange(p.iso);
      text = fmt.format(p.iso);
    } else {
      text = fmt.format(value);
    }
  }

  function onBlur() {
    focused = false;
    commit();
  }

  function choose(iso: string) {
    text = fmt.format(iso);
    onview(iso);
    onchange(iso);
    open = false;
    inputEl?.focus();
  }

  function clear() {
    text = "";
    onchange(null);
    open = false;
    inputEl?.focus();
  }

  $effect(() => {
    if (!open) return;
    function onDocClick(e: MouseEvent) {
      if (wrapEl && !wrapEl.contains(e.target as Node)) {
        open = false;
      }
    }
    window.addEventListener("mousedown", onDocClick);
    return () => window.removeEventListener("mousedown", onDocClick);
  });
</script>

<div class="picker-wrap" bind:this={wrapEl}>
  <input
    bind:this={inputEl}
    {id}
    type="text"
    class="input picker-input"
    class:error={!!error}
    readonly={readOnly}
    {placeholder}
    value={text}
    inputmode="numeric"
    autocomplete="off"
    spellcheck={false}
    data-fieldname={field.fieldname}
    data-fieldtype={fieldtype}
    onfocus={() => (focused = true)}
    onclick={() => { if (!readOnly && !open) togglePicker(); }}
    oninput={onInput}
    onblur={onBlur}
    onkeydown={onKeyDown}
  />
  {#if !readOnly}
    <button
      type="button"
      class="cal-btn"
      tabindex="-1"
      aria-label={openLabel}
      onmousedown={(e) => e.preventDefault()}
      onclick={togglePicker}
    >
      <Icon name="calendar" size={15} />
    </button>
  {/if}

  {#if open && !readOnly}
    <div class="popover" class:compact use:anchored={{ anchor: wrapEl, align: "end" }} role="dialog" aria-modal="true" tabindex="-1" onmousedown={(e) => e.preventDefault()}>
      <div class="popover-head">
        <button type="button" class="btn icon sm" onclick={nav.onprev} aria-label={nav.prevLabel}>
          <Icon name="chevron-left" size={13} />
        </button>
        <span class={compact ? "view-year" : "view-title"}>{nav.title}</span>
        <button type="button" class="btn icon sm" onclick={nav.onnext} aria-label={nav.nextLabel}>
          <Icon name="chevron-right" size={13} />
        </button>
      </div>

      {@render grid(choose)}

      <div class="popover-foot">
        <button type="button" class="btn-link" onclick={() => choose(todayValue)}>
          {todayLabel}
        </button>
        {#if value}
          <button type="button" class="btn-link muted" onclick={clear}>
            {__("Clear")}
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .picker-wrap {
    position: relative;
    width: 100%;
  }

  .picker-input {
    padding-right: 32px;
    font-variant-numeric: tabular-nums;
  }

  .cal-btn {
    position: absolute;
    right: 7px;
    top: 50%;
    transform: translateY(-50%);
    border: none;
    background: transparent;
    cursor: pointer;
    padding: 3px;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--muted);
    border-radius: 4px;
    transition: color 0.15s ease, background-color 0.15s ease;
  }

  .cal-btn:hover {
    color: var(--text);
    background: var(--hover);
  }

  .popover {
    position: fixed;
    z-index: 80;
    overflow: auto;
    background: var(--surface);
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.12);
    padding: 12px;
    width: 250px;
    user-select: none;
  }

  .popover.compact {
    padding: 10px;
    width: 210px;
  }

  .popover-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  .view-title,
  .view-year {
    font-weight: 600;
    font-size: 13px;
    color: var(--text);
  }

  .view-year {
    font-size: 14px;
  }

  .popover-foot {
    display: flex;
    justify-content: space-between;
    align-items: center;
    border-top: 1px solid var(--border);
    padding-top: 6px;
  }

  .btn-link {
    background: none;
    border: none;
    font-size: 11px;
    color: var(--primary);
    cursor: pointer;
    padding: 2px 4px;
    border-radius: 4px;
  }

  .btn-link:hover {
    text-decoration: underline;
  }

  .btn-link.muted {
    color: var(--muted);
  }
</style>
