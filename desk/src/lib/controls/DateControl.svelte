<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import { today } from "$lib/datetime";
  import type { Field } from "$lib/meta";
  import MaskedPickerInput from "./MaskedPickerInput.svelte";
  import {
    dayNames,
    monthTitles,
    datePlaceholder,
    formatDateLocal,
    getCalendarDays,
    maskDateInput,
    parseDateLocal,
    stepDate,
  } from "./date-format.ts";

  let {
    field,
    value,
    onchange,
    readOnly = false,
    error = "",
    id = "",
  }: {
    field: Field;
    value: any;
    onchange: (v: any) => void;
    readOnly?: boolean;
    error?: string;
    id?: string;
  } = $props();

  const fmt = { format: formatDateLocal, parse: parseDateLocal, mask: maskDateInput, step: stepDate, digits: 8 };
  const currentParsed = $derived(parseDateLocal(value));

  // the site's day, not the browser's: the calendar highlighted a "today" the
  // server would not have agreed with for any reader in another timezone
  const todayIso = today();

  let viewYear = $state(Number(todayIso.slice(0, 4)));
  let viewMonth = $state(Number(todayIso.slice(5, 7)));

  const calendarDays = $derived(getCalendarDays(viewYear, viewMonth));

  function showMonth(iso: string) {
    viewYear = Number(iso.slice(0, 4));
    viewMonth = Number(iso.slice(5, 7));
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
</script>

<MaskedPickerInput
  {field}
  fieldtype="Date"
  {value}
  {onchange}
  {readOnly}
  {error}
  {id}
  {fmt}
  placeholder={datePlaceholder()}
  {todayIso}
  todayValue={todayIso}
  todayLabel={__("Today")}
  openLabel={__("Open the date picker")}
  nav={{
    title: `${monthTitles()[viewMonth - 1]} ${viewYear}`,
    prevLabel: __("Previous month"),
    nextLabel: __("Next month"),
    onprev: prevMonth,
    onnext: nextMonth,
  }}
  onview={showMonth}
>
  {#snippet grid(choose)}
    <div class="week-row">
      {#each dayNames() as dayName}
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
          onclick={() => choose(d.iso)}
        >
          {d.day}
        </button>
      {/each}
    </div>
  {/snippet}
</MaskedPickerInput>

<style>
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
    background: var(--hover);
  }

  .day-cell.other-month {
    color: color-mix(in srgb, var(--muted) 50%, var(--surface));
  }

  .day-cell.is-today {
    border-color: var(--border);
    font-weight: 600;
  }

  .day-cell.selected {
    background: var(--primary);
    color: var(--on-primary);
    font-weight: 600;
  }
</style>
