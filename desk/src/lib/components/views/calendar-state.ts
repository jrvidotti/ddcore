import { fromDatetimeLocal, toDatetimeLocal } from "../../datetime";
import type { Field } from "../../meta";

export function isCalendarDatetime(fields: Field[], field: string): boolean {
  if (field === "creation" || field === "modified") return true;
  return fields.find((f) => f.fieldname === field)?.fieldtype === "Datetime";
}

/** The site day (YYYY-MM-DD) a row's Date or Datetime field falls on, or "" without a valid value. */
export function calendarDay(row: Record<string, any>, field: string, fields: Field[], tz?: string): string {
  const value = row[field];
  const iso = isCalendarDatetime(fields, field) ? toDatetimeLocal(value, tz).slice(0, 10) : String(value ?? "").slice(0, 10);
  return /^\d{4}-\d{2}-\d{2}$/.test(iso) ? iso : "";
}

/** Groups rows by day; a row without a date goes on `undatedOn` when one is given, else nowhere. */
export function groupCalendarRows<T extends Record<string, any>>(rows: T[], field: string, fields: Field[], tz?: string, undatedOn?: string): Map<string, T[]> {
  const groups = new Map<string, T[]>();
  for (const row of rows) {
    const iso = calendarDay(row, field, fields, tz) || undatedOn;
    if (!iso) continue;
    const group = groups.get(iso) || [];
    group.push(row);
    groups.set(iso, group);
  }
  return groups;
}

export function calendarRangeFilters(field: string, fields: Field[], startIso: string, endIso: string, tz?: string): any[][] {
  const isDatetime = isCalendarDatetime(fields, field);
  // Postgres timestamps have microsecond precision; include the final day's
  // last instant after converting its wall-clock time in the site's zone.
  const rangeStart = isDatetime ? fromDatetimeLocal(`${startIso}T00:00`, tz) : startIso;
  const rangeEnd = isDatetime ? fromDatetimeLocal(`${endIso}T23:59`, tz)?.replace(":00.000Z", ":59.999999Z") : endIso;
  return [[field, ">=", rangeStart], [field, "<=", rangeEnd]];
}

/**
 * Rows that overlap the grid when records span `field` → `endField`, as two
 * disjoint filter sets: rows with an end that overlap it, and rows without an
 * end that start in it. Two sets because the API has a single OR group, which
 * the search already uses.
 */
export function calendarSpanFilters(field: string, endField: string, fields: Field[], startIso: string, endIso: string, tz?: string): any[][][] {
  const [startsAfterGridStart, startsBeforeGridEnd] = calendarRangeFilters(field, fields, startIso, endIso, tz);
  const [endsAfterGridStart] = calendarRangeFilters(endField, fields, startIso, endIso, tz);
  return [
    [startsBeforeGridEnd, endsAfterGridStart],
    [startsAfterGridStart, startsBeforeGridEnd, [endField, "is", "not set"]],
  ];
}

export interface CalendarSegment<T> {
  row: T;
  /** The record's first and last day fall here (not a week clip): its rounded ends. */
  start: boolean;
  end: boolean;
  /** The piece that shows the title: the first one in each week. */
  label: boolean;
}

/**
 * Lays rows out on the grid's days, one lane per row within each week, so a
 * record spanning `field` → `endField` keeps its row across the days it
 * covers. Each day gets its lanes in order, `null` where a lane is empty.
 * Without `endField` (or its value) a row takes its start day alone.
 */
export function calendarLanes<T extends Record<string, any>>(
  rows: T[],
  opts: { field: string; endField?: string },
  fields: Field[],
  days: { iso: string }[],
  tz?: string,
  undatedOn?: string,
): Map<string, (CalendarSegment<T> | null)[]> {
  const spans: { row: T; start: string; end: string }[] = [];
  for (const row of rows) {
    const start = calendarDay(row, opts.field, fields, tz) || undatedOn;
    if (!start) continue;
    const end = opts.endField ? calendarDay(row, opts.endField, fields, tz) : "";
    spans.push({ row, start, end: end && end > start ? end : start });
  }
  const out = new Map<string, (CalendarSegment<T> | null)[]>();
  for (let w = 0; w < days.length; w += 7) {
    const week = days.slice(w, w + 7).map((d) => d.iso);
    const first = week[0], last = week[week.length - 1];
    const pieces = spans
      .filter((s) => s.start <= last && s.end >= first)
      .map((s) => ({ ...s, from: Math.max(0, week.indexOf(s.start)), to: s.end > last ? week.length - 1 : week.indexOf(s.end) }))
      .sort((a, b) => a.from - b.from || b.to - a.to);
    const lanes: (CalendarSegment<T> | null)[][] = week.map(() => []);
    for (const p of pieces) {
      let lane = 0;
      while (week.some((_, i) => i >= p.from && i <= p.to && lanes[i][lane])) lane++;
      for (let i = p.from; i <= p.to; i++) {
        while (lanes[i].length < lane) lanes[i].push(null);
        lanes[i][lane] = { row: p.row, start: week[i] === p.start, end: week[i] === p.end, label: i === p.from };
      }
    }
    // gaps only matter under a later lane: trailing empties would just add height
    week.forEach((iso, i) => {
      const slots = Array.from(lanes[i], (s) => s ?? null);
      while (slots.length && !slots[slots.length - 1]) slots.pop();
      if (slots.length) out.set(iso, slots);
    });
  }
  return out;
}
