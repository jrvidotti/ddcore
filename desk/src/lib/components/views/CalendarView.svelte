<script lang="ts">
  import { untrack } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import type { CalendarViewOptions } from "$lib/desk-sdk";
  import type { Meta } from "$lib/meta";
  import { dayNames, getCalendarDays, monthTitles } from "$lib/controls/date-format";
  import { today, toDatetimeLocal } from "$lib/datetime";
  import { statusColor } from "$lib/format";
  import Icon from "../Icon.svelte";

  let { rows, meta, doctype, wsPrefix, calendar, viewYear = Number(today().slice(0, 4)), viewMonth = Number(today().slice(5, 7)), onMonthChange }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; calendar: CalendarViewOptions;
    viewYear?: number; viewMonth?: number;
    onMonthChange: (year: number, month: number, gridStartIso: string, gridEndIso: string) => void;
  } = $props();

  const days = $derived(getCalendarDays(viewYear, viewMonth));
  const titleField = $derived(calendar.titleField || meta.doctype.titleField || "name");
  const colorField = $derived(meta.doctype.fields.find((f) => f.fieldname === (calendar.colorField || "status")));
  const dateField = $derived(meta.doctype.fields.find((f) => f.fieldname === calendar.field));
  const todayIso = $derived(today());
  const byDay = $derived.by(() => {
    const groups = new Map<string, any[]>();
    for (const row of rows) {
      const value = row[calendar.field];
      const iso = dateField?.fieldtype === "Datetime" ? toDatetimeLocal(value).slice(0, 10) : String(value ?? "").slice(0, 10);
      if (!/^\d{4}-\d{2}-\d{2}$/.test(iso)) continue;
      const group = groups.get(iso) || [];
      group.push(row);
      groups.set(iso, group);
    }
    return groups;
  });

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
          {#if meta.permissions.create}
            <a class="day-number create" href={`${wsPrefix}/${encodeURIComponent(doctype)}/new?${encodeURIComponent(calendar.field)}=${day.iso}`} aria-label={`${__("New")} — ${day.iso}`} title={`${__("New")} — ${day.iso}`}>{day.day}</a>
          {:else}<span class="day-number">{day.day}</span>{/if}
          <div class="events">
            {#each byDay.get(day.iso) || [] as row (row.name)}
              <a class="indicator event {statusColor(row[calendar.colorField || "status"], colorField)}" href={`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(row.name)}`} title={String(row[titleField] || row.name)}>{row[titleField] || row.name}</a>
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
  .create::after { content: ""; position: absolute; inset: 0; }
  .create:hover::after { background: rgba(37, 99, 235, .04); }
  .create:focus-visible::after { outline: 2px solid var(--primary); outline-offset: -2px; }
  .events { display: grid; gap: 4px; margin-top: 6px; }
  .event { position: relative; z-index: 1; display: block; padding: 4px 6px; border-radius: 4px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .event::before { display: none; }
  .event:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  @media (max-width: 480px) { .calendar-head { flex-wrap: wrap; } }
</style>
