<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import type { Field } from "$lib/meta";
  import Icon from "$lib/components/Icon.svelte";
  import {
    MONTH_NAMES_SHORT,
    formatMonth,
    maskMonthInput,
    parseMonth,
  } from "./month-format";

  let {
    field,
    value,
    onchange,
    readOnly = false,
    error = "",
    id = "",
    inGrid = false,
  }: {
    field: Field;
    value: any;
    onchange: (v: any) => void;
    readOnly?: boolean;
    error?: string;
    id?: string;
    inGrid?: boolean;
  } = $props();

  let text = $state("");
  let focused = $state(false);
  let open = $state(false);
  let wrapEl: HTMLDivElement | null = $state(null);
  let inputEl: HTMLInputElement | null = $state(null);

  const currentParsed = $derived(parseMonth(value));
  let viewYear = $state(new Date().getFullYear());

  const now = new Date();
  const todayYear = now.getFullYear();
  const todayMonth = now.getMonth() + 1;

  $effect(() => {
    if (!focused) {
      text = formatMonth(value);
    }
  });

  function togglePicker() {
    if (readOnly) return;
    if (!open) {
      viewYear = currentParsed ? currentParsed.year : todayYear;
      open = true;
    } else {
      open = false;
    }
  }

  function onInput(e: Event) {
    const raw = (e.target as HTMLInputElement).value;
    text = maskMonthInput(raw);
    if (text.length === 7) {
      const p = parseMonth(text);
      if (p) onchange(p.iso);
    }
  }

  function onKeyDown(e: KeyboardEvent) {
    if (e.key === "Backspace") {
      const target = e.target as HTMLInputElement;
      if (
        text.endsWith("/") &&
        target.selectionStart === target.selectionEnd &&
        target.selectionStart === text.length
      ) {
        e.preventDefault();
        text = text.slice(0, -2);
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
    }
  }

  function commit() {
    if (!text) {
      if (value) onchange(null);
      return;
    }
    const p = parseMonth(text);
    if (p) {
      onchange(p.iso);
      text = formatMonth(p.iso);
    } else {
      text = formatMonth(value);
    }
  }

  function onBlur() {
    focused = false;
    commit();
  }

  function pickMonth(m: number) {
    const padM = String(m).padStart(2, "0");
    const iso = `${viewYear}-${padM}-01`;
    text = `${padM}/${viewYear}`;
    onchange(iso);
    open = false;
    inputEl?.focus();
  }

  function pickThisMonth() {
    const padM = String(todayMonth).padStart(2, "0");
    const iso = `${todayYear}-${padM}-01`;
    viewYear = todayYear;
    text = `${padM}/${todayYear}`;
    onchange(iso);
    open = false;
    inputEl?.focus();
  }

  function clearMonth() {
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

<div class="month-wrap" bind:this={wrapEl}>
  <input
    bind:this={inputEl}
    {id}
    type="text"
    class="input month-input"
    class:error={!!error}
    readonly={readOnly}
    placeholder="mm/aaaa"
    value={text}
    inputmode="numeric"
    autocomplete="off"
    spellcheck={false}
    data-fieldname={field.fieldname}
    data-fieldtype="Month"
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
      aria-label={__("Open the month picker")}
      onmousedown={(e) => e.preventDefault()}
      onclick={togglePicker}
    >
      <Icon name="calendar" size={15} />
    </button>
  {/if}

  {#if open && !readOnly}
    <div class="month-popover" role="dialog" aria-modal="true" tabindex="-1" onmousedown={(e) => e.preventDefault()}>
      <div class="popover-head">
        <button
          type="button"
          class="btn icon sm"
          onclick={() => (viewYear -= 1)}
          aria-label={__("Previous year")}
        >
          <Icon name="chevron-left" size={13} />
        </button>
        <span class="view-year">{viewYear}</span>
        <button
          type="button"
          class="btn icon sm"
          onclick={() => (viewYear += 1)}
          aria-label={__("Next year")}
        >
          <Icon name="chevron-right" size={13} />
        </button>
      </div>

      <div class="popover-grid">
        {#each MONTH_NAMES_SHORT as name, idx}
          {@const mNum = idx + 1}
          {@const isSelected =
            currentParsed?.year === viewYear && currentParsed?.month === mNum}
          {@const isToday =
            todayYear === viewYear && todayMonth === mNum}
          <button
            type="button"
            class="month-cell"
            class:selected={isSelected}
            class:is-today={isToday && !isSelected}
            onclick={() => pickMonth(mNum)}
          >
            {name}
          </button>
        {/each}
      </div>

      <div class="popover-foot">
        <button type="button" class="btn-link" onclick={pickThisMonth}>
          {__("This month")}
        </button>
        {#if value}
          <button type="button" class="btn-link muted" onclick={clearMonth}>
            Limpar
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .month-wrap {
    position: relative;
    width: 100%;
  }

  .month-input {
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
    background: #f3f4f6;
  }

  .month-popover {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 50;
    background: #fff;
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.12);
    padding: 10px;
    width: 210px;
    user-select: none;
  }

  .popover-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  .view-year {
    font-weight: 600;
    font-size: 14px;
    color: var(--text);
  }

  .popover-grid {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 5px;
    margin-bottom: 8px;
  }

  .month-cell {
    border: 1px solid transparent;
    background: transparent;
    border-radius: 6px;
    padding: 6px 0;
    font-size: 13px;
    text-align: center;
    cursor: pointer;
    color: var(--text);
    transition: all 0.1s ease;
  }

  .month-cell:hover {
    background: #f3f4f6;
  }

  .month-cell.is-today {
    border-color: var(--border);
    font-weight: 500;
  }

  .month-cell.selected {
    background: var(--primary);
    color: #fff;
    font-weight: 600;
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
