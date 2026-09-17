<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import type { GanttViewOptions } from "$lib/desk-sdk";
  import type { Meta } from "$lib/meta";
  import { statusColor } from "$lib/format";
  import { today } from "$lib/datetime";
  import { monthNames } from "$lib/locale";
  import Icon from "../Icon.svelte";
  import { dayDiff, ganttBar, ganttWindow, shiftAnchor, type GanttScale } from "./gantt-state";

  let { rows, meta, doctype, wsPrefix, gantt, scale, anchor, onChange }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; gantt: GanttViewOptions;
    scale: GanttScale; anchor: string;
    onChange: (scale: GanttScale, anchor: string) => void;
  } = $props();

  const win = $derived(ganttWindow(anchor, scale));
  const titleField = $derived(gantt.titleField || meta.doctype.titleField || "name");
  const colorField = $derived(meta.doctype.fields.find((f) => f.fieldname === (gantt.colorField || "status")));
  const bars = $derived(rows
    .map((row) => ({ row, bar: ganttBar(row, gantt, meta.doctype.fields, win) }))
    .filter((x) => x.bar));
  const todayIso = today();
  const todayOffset = $derived(todayIso >= win.start && todayIso <= win.end ? ((dayDiff(win.start, todayIso) + 0.5) / win.days) * 100 : null);
  const scales: [GanttScale, string][] = $derived([["day", __("Day")], ["week", __("Week")], ["month", __("Month")]]);

  const isWeekend = (start: string) => scale === "day" && [0, 6].includes(new Date(start + "T00:00:00Z").getUTCDay());
  function columnLabel(start: string): string {
    const m = monthNames("short")[Number(start.slice(5, 7)) - 1];
    if (scale === "month") return `${m} ${start.slice(2, 4)}`;
    if (scale === "week") return `${Number(start.slice(8, 10))} ${m}`;
    return String(Number(start.slice(8, 10)));
  }
  const title = $derived(`${monthNames("long")[Number(win.start.slice(5, 7)) - 1]} ${win.start.slice(0, 4)} – ${monthNames("long")[Number(win.end.slice(5, 7)) - 1]} ${win.end.slice(0, 4)}`);
</script>

<div class="card gantt">
  <div class="gantt-head">
    <h2 aria-live="polite">{title}</h2>
    <div class="scale" role="group" aria-label={__("Scale")}>
      {#each scales as [value, label]}
        <button class="btn sm" class:active={scale === value} aria-pressed={scale === value} onclick={() => onChange(value, anchor)}>{label}</button>
      {/each}
    </div>
    <div class="navigation">
      <button class="btn" aria-label={__("Previous")} title={__("Previous")} onclick={() => onChange(scale, shiftAnchor(anchor, scale, -1))}><Icon name="chevron-left" size={16} /></button>
      <button class="btn" onclick={() => onChange(scale, today())}>{__("Today")}</button>
      <button class="btn" aria-label={__("Next")} title={__("Next")} onclick={() => onChange(scale, shiftAnchor(anchor, scale, 1))}><Icon name="chevron-right" size={16} /></button>
    </div>
  </div>
  <div class="gantt-scroll">
    <div class="gantt-grid">
      <div class="label-col head"></div>
      <div class="timeline head">
        {#each win.columns as col (col.start)}
          <div class="tick" style:width={`${(col.days / win.days) * 100}%`} class:weekend={isWeekend(col.start)}>{columnLabel(col.start)}</div>
        {/each}
      </div>
      {#each bars as { row, bar } (row.name)}
        {@const href = `${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(row.name)}`}
        <a class="label-col row-label" {href} title={String(row[titleField] || row.name)}>{row[titleField] || row.name}</a>
        <div class="timeline">
          {#each win.columns as col (col.start)}<div class="cell" class:weekend={isWeekend(col.start)} style:width={`${(col.days / win.days) * 100}%`}></div>{/each}
          {#if todayOffset !== null}<div class="today" style:left={`${todayOffset}%`}></div>{/if}
          <a
            class="bar indicator {statusColor(row[gantt.colorField || "status"], colorField)}"
            class:clipped-start={bar!.clippedStart}
            class:clipped-end={bar!.clippedEnd}
            {href}
            style:left={`${bar!.left}%`}
            style:width={`${bar!.width}%`}
            title={`${row[titleField] || row.name}: ${bar!.start} → ${bar!.end}`}
          >
            {#if bar!.progress !== null}<span class="progress" style:width={`${bar!.progress}%`}></span>{/if}
            <span class="bar-label">{row[titleField] || row.name}</span>
          </a>
        </div>
      {:else}
        <div class="empty muted">{__("No records in this period")}</div>
      {/each}
    </div>
  </div>
</div>

<style>
  .gantt { overflow: hidden; }
  .gantt-head { display: flex; align-items: center; gap: 12px; padding: 14px; flex-wrap: wrap; }
  h2 { font-size: 16px; font-weight: 600; margin: 0; flex: 1; }
  .scale, .navigation { display: flex; gap: 6px; }
  .scale .active { color: var(--primary); border-color: var(--primary); background: var(--bg); }
  .gantt-scroll { overflow-x: auto; }
  .gantt-grid { display: grid; grid-template-columns: 200px 1fr; min-width: 820px; }
  .label-col { padding: 6px 12px; border-top: 1px solid var(--border); border-right: 1px solid var(--border); overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 13px; }
  .row-label { color: var(--text); }
  .timeline { position: relative; display: flex; border-top: 1px solid var(--border); min-height: 34px; }
  .timeline.head { min-height: 0; }
  .tick { font-size: 11px; color: var(--muted); padding: 6px 4px; text-align: center; border-left: 1px solid var(--border); overflow: hidden; white-space: nowrap; }
  .tick.weekend, .cell.weekend { background: var(--bg); }
  .cell { border-left: 1px solid var(--border); }
  .today { position: absolute; top: 0; bottom: 0; width: 2px; background: var(--primary); opacity: .4; }
  .bar { position: absolute; top: 6px; height: 22px; padding: 0 8px; border-radius: 4px; overflow: hidden; display: flex; align-items: center; text-decoration: none; min-width: 6px; }
  .bar::before { display: none; }
  .bar.clipped-start { border-top-left-radius: 0; border-bottom-left-radius: 0; }
  .bar.clipped-end { border-top-right-radius: 0; border-bottom-right-radius: 0; }
  .bar:hover { text-decoration: none; filter: brightness(.96); }
  .progress { position: absolute; left: 0; top: 0; bottom: 0; background: currentColor; opacity: .18; }
  .bar-label { position: relative; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; font-size: 12px; }
  .empty { grid-column: 1 / -1; padding: 24px; text-align: center; border-top: 1px solid var(--border); }
</style>
