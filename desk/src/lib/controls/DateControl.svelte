<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import type { Field } from "$lib/meta";
  import Icon from "$lib/components/Icon.svelte";
  import {
    DAY_NAMES_SHORT,
    MONTH_NAMES,
    formatDateBr,
    getCalendarDays,
    maskDateInput,
    parseDateBr,
    type CalendarDay,
  } from "./date-format.ts";

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

  const currentParsed = $derived(parseDateBr(value));

  const now = new Date();
  const todayYear = now.getFullYear();
  const todayMonth = now.getMonth() + 1;
  const todayDay = now.getDate();
  const todayIso = `${todayYear}-${String(todayMonth).padStart(2, "0")}-${String(todayDay).padStart(2, "0")}`;

  let viewYear = $state(todayYear);
  let viewMonth = $state(todayMonth);

  const calendarDays = $derived(getCalendarDays(viewYear, viewMonth));

  $effect(() => {
    if (!focused) {
      text = formatDateBr(value);
    }
  });

  function togglePicker() {
    if (readOnly) return;
    if (!open) {
      if (currentParsed) {
        viewYear = currentParsed.year;
        viewMonth = currentParsed.month;
      } else {
        viewYear = todayYear;
        viewMonth = todayMonth;
      }
      open = true;
    } else {
      open = false;
    }
  }

  function prevMonth() {
    if (viewMonth === 1) {
      viewMonth = 12;
      viewYear -= 1;
    } else {
      viewMonth -= 1;
    }
  }

  function nextMonth() {
    if (viewMonth === 12) {
      viewMonth = 1;
      viewYear += 1;
    } else {
      viewMonth += 1;
    }
  }

  function onInput(e: Event) {
    const raw = (e.target as HTMLInputElement).value;
    text = maskDateInput(raw);
    if (text.length === 10) {
      const p = parseDateBr(text);
      if (p) {
        viewYear = p.year;
        viewMonth = p.month;
        onchange(p.iso);
      }
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
    const p = parseDateBr(text);
    if (p) {
      onchange(p.iso);
      text = formatDateBr(p.iso);
    } else {
      text = formatDateBr(value);
    }
  }

  function onBlur() {
    focused = false;
    commit();
  }

  function pickDay(d: CalendarDay) {
    text = formatDateBr(d.iso);
    viewYear = d.year;
    viewMonth = d.month;
    onchange(d.iso);
    open = false;
    inputEl?.focus();
  }

  function pickToday() {
    text = formatDateBr(todayIso);
    viewYear = todayYear;
    viewMonth = todayMonth;
    onchange(todayIso);
    open = false;
    inputEl?.focus();
  }

  function clearDate() {
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

<div class="date-wrap" bind:this={wrapEl}>
  <input
    bind:this={inputEl}
    {id}
    type="text"
    class="input date-input"
    class:error={!!error}
    readonly={readOnly}
    placeholder="dd/mm/aaaa"
    value={text}
    inputmode="numeric"
    autocomplete="off"
    spellcheck={false}
    data-fieldname={field.fieldname}
    data-fieldtype="Date"
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
      aria-label="Abrir seletor de data"
      onmousedown={(e) => e.preventDefault()}
      onclick={togglePicker}
    >
      <Icon name="calendar" size={15} />
    </button>
  {/if}

  {#if open && !readOnly}
    <div class="date-popover" role="dialog" aria-modal="true" tabindex="-1" onmousedown={(e) => e.preventDefault()}>
      <div class="popover-head">
        <button
          type="button"
          class="btn icon sm"
          onclick={prevMonth}
          aria-label={__("Previous month")}
        >
          <Icon name="chevron-left" size={13} />
        </button>
        <span class="view-title">{MONTH_NAMES[viewMonth - 1]} {viewYear}</span>
        <button
          type="button"
          class="btn icon sm"
          onclick={nextMonth}
          aria-label={__("Next month")}
        >
          <Icon name="chevron-right" size={13} />
        </button>
      </div>

      <div class="week-row">
        {#each DAY_NAMES_SHORT as dayName}
          <div class="week-day">{dayName}</div>
        {/each}
      </div>

      <div class="days-grid">
        {#each calendarDays as d}
          {@const isSelected = currentParsed?.iso === d.iso}
          {@const isToday = todayIso === d.iso}
          <button
            type="button"
            class="day-cell"
            class:other-month={!d.isCurrentMonth}
            class:selected={isSelected}
            class:is-today={isToday && !isSelected}
            onclick={() => pickDay(d)}
          >
            {d.day}
          </button>
        {/each}
      </div>

      <div class="popover-foot">
        <button type="button" class="btn-link" onclick={pickToday}>
          Hoje
        </button>
        {#if value}
          <button type="button" class="btn-link muted" onclick={clearDate}>
            Limpar
          </button>
        {/if}
      </div>
    </div>
  {/if}
</div>

<style>
  .date-wrap {
    position: relative;
    width: 100%;
  }

  .date-input {
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

  .date-popover {
    position: absolute;
    top: calc(100% + 4px);
    right: 0;
    z-index: 50;
    background: #fff;
    border: 1px solid var(--border);
    border-radius: 8px;
    box-shadow: 0 10px 25px rgba(0, 0, 0, 0.12);
    padding: 12px;
    width: 250px;
    user-select: none;
  }

  .popover-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }

  .view-title {
    font-weight: 600;
    font-size: 13px;
    color: var(--text);
  }

  .week-row {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    margin-bottom: 4px;
  }

  .week-day {
    font-size: 11px;
    font-weight: 500;
    color: var(--muted);
    text-align: center;
    padding: 2px 0;
  }

  .days-grid {
    display: grid;
    grid-template-columns: repeat(7, 1fr);
    gap: 2px;
    margin-bottom: 8px;
  }

  .day-cell {
    border: 1px solid transparent;
    background: transparent;
    border-radius: 6px;
    height: 28px;
    font-size: 12px;
    text-align: center;
    cursor: pointer;
    color: var(--text);
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.1s ease;
    padding: 0;
  }

  .day-cell:hover {
    background: #f3f4f6;
  }

  .day-cell.other-month {
    color: #cbd5e1;
  }

  .day-cell.is-today {
    border-color: var(--border);
    font-weight: 600;
  }

  .day-cell.selected {
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
