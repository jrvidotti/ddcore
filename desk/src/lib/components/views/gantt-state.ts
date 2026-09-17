// Pure logic behind the Gantt view: the date window a scale shows, the query
// that loads the rows overlapping it, and where each bar sits.
import { addDays, addMonths, monthStart, toDatetimeLocal } from "../../datetime";
import type { Field } from "../../meta";
import { calendarRangeFilters, isCalendarDatetime } from "./calendar-state";

export type GanttScale = "day" | "week" | "month";

export interface GanttColumn {
  /** First day of the column. */
  start: string;
  /** Number of days the column spans. */
  days: number;
}

export interface GanttWindow {
  start: string;
  /** Last day shown, inclusive. */
  end: string;
  days: number;
  columns: GanttColumn[];
}

export interface GanttBar {
  /** Offset from the window start, in percent of its width. */
  left: number;
  width: number;
  start: string;
  end: string;
  /** The bar goes on before or after the window. */
  clippedStart: boolean;
  clippedEnd: boolean;
  /** 0–100, or null when the view has no progress field or the row no value. */
  progress: number | null;
}

const COLUMNS: Record<GanttScale, number> = { day: 21, week: 12, month: 12 };
/** How far one "previous"/"next" moves, in columns. */
const STEP: Record<GanttScale, number> = { day: 7, week: 4, month: 3 };

/** Days from a to b (both YYYY-MM-DD). */
export function dayDiff(a: string, b: string): number {
  const utc = (s: string) => Date.UTC(Number(s.slice(0, 4)), Number(s.slice(5, 7)) - 1, Number(s.slice(8, 10)));
  return Math.round((utc(b) - utc(a)) / 86400000);
}

/** Sunday of the week holding d, matching the calendar's first weekday. */
export function weekStart(d: string): string {
  const dow = new Date(Date.UTC(Number(d.slice(0, 4)), Number(d.slice(5, 7)) - 1, Number(d.slice(8, 10)))).getUTCDay();
  return addDays(d, -dow);
}

/**
 * The window a scale shows around an anchor day: it starts one week (or one
 * month) before the anchor's, so work that began just before it stays in view.
 */
export function ganttWindow(anchor: string, scale: GanttScale): GanttWindow {
  const columns: GanttColumn[] = [];
  let cursor = scale === "month" ? addMonths(monthStart(anchor), -1) : addDays(weekStart(anchor), -7);
  for (let i = 0; i < COLUMNS[scale]; i++) {
    const next = scale === "day" ? addDays(cursor, 1) : scale === "week" ? addDays(cursor, 7) : addMonths(cursor, 1);
    columns.push({ start: cursor, days: dayDiff(cursor, next) });
    cursor = next;
  }
  const start = columns[0].start;
  const end = addDays(cursor, -1);
  return { start, end, days: dayDiff(start, cursor), columns };
}

/** The anchor after moving one step back (-1) or forward (1). */
export function shiftAnchor(anchor: string, scale: GanttScale, direction: number): string {
  const n = STEP[scale] * direction;
  if (scale === "month") return addMonths(monthStart(anchor), n);
  return addDays(anchor, scale === "week" ? n * 7 : n);
}

/** Rows whose span overlaps the window: they start before it ends and end after it starts. */
export function ganttRangeFilters(startField: string, endField: string, fields: Field[], win: GanttWindow, tz?: string): any[][] {
  const [, startsBeforeEnd] = calendarRangeFilters(startField, fields, win.start, win.end, tz);
  const [endsAfterStart] = calendarRangeFilters(endField, fields, win.start, win.end, tz);
  return [startsBeforeEnd, endsAfterStart];
}

function dayOf(row: Record<string, any>, field: string, fields: Field[], tz?: string): string {
  const v = row[field];
  const iso = isCalendarDatetime(fields, field) ? toDatetimeLocal(v, tz).slice(0, 10) : String(v ?? "").slice(0, 10);
  return /^\d{4}-\d{2}-\d{2}$/.test(iso) ? iso : "";
}

/** Where a row's bar sits in the window, or null when it has no span there. */
export function ganttBar(
  row: Record<string, any>,
  opts: { startField: string; endField: string; progressField?: string },
  fields: Field[],
  win: GanttWindow,
  tz?: string,
): GanttBar | null {
  const start = dayOf(row, opts.startField, fields, tz);
  let end = dayOf(row, opts.endField, fields, tz);
  if (!start || !end) return null;
  if (end < start) end = start;
  if (end < win.start || start > win.end) return null;
  const from = start < win.start ? win.start : start;
  const to = end > win.end ? win.end : end;
  let progress: number | null = null;
  if (opts.progressField) {
    const p = Number(row[opts.progressField]);
    if (row[opts.progressField] !== null && row[opts.progressField] !== undefined && row[opts.progressField] !== "" && Number.isFinite(p)) {
      progress = Math.min(100, Math.max(0, p));
    }
  }
  return {
    left: (dayDiff(win.start, from) / win.days) * 100,
    width: ((dayDiff(from, to) + 1) / win.days) * 100,
    start,
    end,
    clippedStart: start < win.start,
    clippedEnd: end > win.end,
    progress,
  };
}
