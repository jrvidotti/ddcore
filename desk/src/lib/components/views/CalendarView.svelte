<script lang="ts">
  import { untrack } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import type { CalendarViewOptions } from "$lib/desk-sdk";
  import type { Meta } from "$lib/meta";
  import { dayNames, getCalendarDays, monthTitles } from "$lib/controls/date-format";
  import { fromDatetimeLocal, today } from "$lib/datetime";
  import { colorStyle, formatDate, statusColor } from "$lib/format";
  import Icon from "../Icon.svelte";
  import { calendarLanes, isCalendarDatetime } from "./calendar-state";

  let { rows, meta, doctype, wsPrefix, basePath, calendar, viewYear = Number(today().slice(0, 4)), viewMonth = Number(today().slice(5, 7)), onMonthChange, onDayClick, undatedOn }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; calendar: CalendarViewOptions;
    /** where the records live; `${wsPrefix}/<doctype>` unless the page has its own routes */
    basePath?: string;
    viewYear?: number; viewMonth?: number;
    onMonthChange: (year: number, month: number, gridStartIso: string, gridEndIso: string) => void;
    /** clicking a day; without it the day is not clickable (its + still creates) */
    onDayClick?: (iso: string) => void;
    /** the day rows without a date are shown on */
    undatedOn?: string;
  } = $props();

  const base = $derived(basePath || `${wsPrefix}/${encodeURIComponent(doctype)}`);

  const days = $derived(getCalendarDays(viewYear, viewMonth));
  const titleField = $derived(calendar.titleField || meta.doctype.titleField || "id");
  const colorField = $derived(meta.doctype.fields.find((f) => f.fieldname === (calendar.colorField || "status")));
  const isDatetime = $derived(isCalendarDatetime(meta.doctype.fields, calendar.field));
  const todayIso = $derived(today());
  const byDay = $derived(calendarLanes(rows, calendar, meta.doctype.fields, days, undefined, undatedOn));

  // Notify on first render too, so the parent fetches overflow days in the grid.
  $effect(() => {
    const year = viewYear, month = viewMonth, start = days[0].iso, end = days[days.length - 1].iso;
    untrack(() => onMonthChange(year, month, start, end));
  });
  function moveMonth(delta: number) {
    const date = new Date(Date.UTC(viewYear, viewMonth - 1 + delta, 1));
    viewYear = date.getUTCFullYear();
    viewMonth = date.getUTCMonth() + 1;
  }
  function showToday() {
    const date = today();
    viewYear = Number(date.slice(0, 4));
    viewMonth = Number(date.slice(5, 7));
  }
</script>

<div class="card calendar">
  <div class="calendar-head">
    <h2 aria-live="polite">{monthTitles()[viewMonth - 1]} {viewYear}</h2>
    <div class="navigation">
      <button class="btn" aria-label={__("Previous month")} title={__("Previous month")} onclick={() => moveMonth(-1)}><Icon name="chevron-left" size={16} /></button>
      <button class="btn" onclick={showToday}>{__("Today")}</button>
      <button class="btn" aria-label={__("Next month")} title={__("Next month")} onclick={() => moveMonth(1)}><Icon name="chevron-right" size={16} /></button>
    </div>
  </div>
  <div class="calendar-scroll">
    <div class="calendar-grid">
      {#each dayNames() as name}<div class="weekday">{name}</div>{/each}
      {#each days as day (day.iso)}
        <div class="day" class:outside={!day.isCurrentMonth} class:today={day.iso === todayIso}>
          {#if onDayClick}
            <button type="button" class="day-number pick" onclick={() => onDayClick(day.iso)} aria-label={formatDate(day.iso)} title={formatDate(day.iso)}>{day.day}</button>
          {:else}<span class="day-number">{day.day}</span>{/if}
          {#if meta.permissions.create}
            {@const prefill = isDatetime ? fromDatetimeLocal(day.iso + "T00:00") : day.iso}
            <a class="btn icon sm day-new" href={`${base}/new?${encodeURIComponent(calendar.field)}=${encodeURIComponent(prefill || day.iso)}`} aria-label={`${__("New")} — ${formatDate(day.iso)}`} title={`${__("New")} — ${formatDate(day.iso)}`}><Icon name="plus" size={14} /></a>
          {/if}
          <div class="events">
            {#each byDay.get(day.iso) || [] as slot, lane (slot?.row.id ?? `gap-${lane}`)}
              {#if slot}
                {@const row = slot.row}
                <!-- a span's later pieces repeat its link for the mouse only -->
                <a class="indicator event {statusColor(row[calendar.colorField || "status"], colorField)}" style={colorStyle(row[calendar.colorField || "status"], colorField)} class:continues-before={!slot.start} class:continues-after={!slot.end}
                  href={`${base}/${encodeURIComponent(row.id)}`} title={String(row[titleField] || row.id)}
                  tabindex={slot.label ? undefined : -1} aria-hidden={slot.label ? undefined : "true"}>{#if slot.label}{row[titleField] || row.id}{:else}&nbsp;{/if}</a>
              {:else}<div class="event-gap" aria-hidden="true"></div>{/if}
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>
</div>

<style>
  .calendar { overflow: hidden; }
  .calendar-head { display: flex; align-items: center; justify-content: space-between; gap: 12px; padding: 14px; }
  h2 { font-size: 16px; font-weight: 600; margin: 0; }
  .navigation { display: flex; gap: 6px; }
  .calendar-scroll { overflow-x: auto; }
  .calendar-grid { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); min-width: 560px; }
  .weekday { padding: 8px; text-align: center; font-size: 12px; font-weight: 500; color: var(--muted); border-top: 1px solid var(--border); }
  .day { position: relative; min-height: 112px; padding: 8px; border-top: 1px solid var(--border); border-right: 1px solid var(--border); min-width: 0; }
  .day:nth-child(7n) { border-right: 0; }
  .outside { background: var(--bg); }
  .outside .day-number { color: var(--muted); }
  .day-number { display: inline-flex; align-items: center; justify-content: center; width: 26px; height: 26px; font-size: 12px; color: var(--text); border-radius: 50%; }
  .today .day-number { background: var(--primary); color: white; font-weight: 600; }
  .pick { border: 0; padding: 0; background: none; font: inherit; cursor: pointer; }
  .pick::after { content: ""; position: absolute; inset: 0; }
  .pick:hover::after { background: rgba(37, 99, 235, .04); }
  .pick:focus-visible { outline: none; }
  .pick:focus-visible::after { outline: 2px solid var(--primary); outline-offset: -2px; }
  /* above the day's click area; shown on hover, always where there is no hover */
  .day-new { position: absolute; top: 6px; right: 6px; z-index: 2; padding: 3px; opacity: 0; }
  .day:hover .day-new, .day-new:focus-visible { opacity: 1; }
  @media (hover: none) { .day-new { opacity: 1; } }
  .events { display: grid; gap: 4px; margin-top: 6px; }
  .event, .event-gap { height: 24px; }
  .event { position: relative; z-index: 1; display: block; padding: 0 6px; line-height: 24px; border-radius: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  /* a span's pieces reach across the day's padding and border into one bar */
  .event.continues-before { margin-left: -8px; border-top-left-radius: 0; border-bottom-left-radius: 0; }
  .event.continues-after { margin-right: -9px; border-top-right-radius: 0; border-bottom-right-radius: 0; }
  .event::before { display: none; }
  .event:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  @media (max-width: 480px) { .calendar-head { flex-wrap: wrap; } }
</style>
