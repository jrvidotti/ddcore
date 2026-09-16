// Date formatting, parsing and masking for a text date input.
//
// Nothing here fixes an order or a separator. Which of day, month and year
// comes first, and what sits between them, is derived from the locale with a
// probe date — hard-coding "dd/mm/yyyy" is the same mistake as hard-coding
// "R$": right in one place and silently wrong everywhere else.
//
// ISO ("YYYY-MM-DD") is always accepted on input, in every locale: it is the
// wire format, and it is what a user pasting from the API will have.

import { dateShape, dayNamesShort, monthNames } from "../locale";

/** Weekday abbreviations for the calendar header, Sunday first. */
export const dayNames = (): string[] => dayNamesShort();

/** Full month names for the calendar's title. */
export const monthTitles = (): string[] => monthNames("long");

interface Shape {
  order: ("day" | "month" | "year")[];
  sep: string;
  widths: number[];
}

function shape(): Shape {
  const { order, sep } = dateShape();
  return { order, sep, widths: order.map((o) => (o === "year" ? 4 : 2)) };
}

/** The hint for the input, e.g. "dd/mm/yyyy" or "mm/dd/yyyy". */
export const datePlaceholder = (): string => dateShape().placeholder;

const ISO = /^(\d{4})-(\d{1,2})-(\d{1,2})$/;

/** Formats an ISO date ("2026-03-01") the way the locale writes it. */
export function formatDateLocal(v: any): string {
  if (!v) return "";
  const s = String(v).trim().slice(0, 10);
  const m = s.match(ISO);
  if (!m) {
    // already localized (or something we cannot read): hand it back untouched
    return s;
  }
  const { order, sep } = shape();
  const part = { year: m[1], month: m[2].padStart(2, "0"), day: m[3].padStart(2, "0") };
  return order.map((o) => part[o]).join(sep);
}

export function daysInMonth(year: number, month: number): number {
  return new Date(year, month, 0).getDate();
}

export interface ParsedDate {
  year: number;
  month: number;
  day: number;
  iso: string; // "YYYY-MM-DD"
}

/** Parses the locale's own order, or ISO, into year, month, day and ISO. */
export function parseDateLocal(text: string): ParsedDate | null {
  if (!text) return null;
  const s = String(text).trim();

  const build = (y: number, m: number, d: number): ParsedDate | null => {
    if (m < 1 || m > 12 || y < 1000 || y > 9999) return null;
    if (d < 1 || d > daysInMonth(y, m)) return null;
    return { year: y, month: m, day: d, iso: `${y}-${String(m).padStart(2, "0")}-${String(d).padStart(2, "0")}` };
  };

  // ISO is accepted everywhere: it is the wire format
  const mIso = s.match(ISO);
  if (mIso) return build(Number(mIso[1]), Number(mIso[2]), Number(mIso[3]));

  const { order, sep } = shape();
  const nums = s.split(sep).map((p) => p.trim());
  if (nums.length !== 3 || nums.some((n) => !/^\d{1,4}$/.test(n))) return null;
  const at = (o: "day" | "month" | "year") => Number(nums[order.indexOf(o)]);
  return build(at("year"), at("month"), at("day"));
}

/**
 * Formats typed digits into the locale's order, inserting the separator as it
 * goes and clamping each part to what a date can hold. The segment widths come
 * from the order, so an ISO-first locale masks 4-2-2 and a day-first one 2-2-4
 * without a second implementation.
 */
export function maskDateInput(raw: string): string {
  if (!raw) return "";
  const { order, sep, widths } = shape();
  const total = widths.reduce((a, b) => a + b, 0);
  const digits = raw.replace(/\D/g, "").slice(0, total);
  if (!digits) return "";

  const max = { day: 31, month: 12, year: 9999 };
  const out: string[] = [];
  let i = 0;
  for (let seg = 0; seg < order.length; seg++) {
    const w = widths[seg];
    const chunk = digits.slice(i, i + w);
    if (!chunk) break;
    i += chunk.length;
    const complete = chunk.length === w;
    if (!complete) {
      // a lone digit that cannot start a valid two-digit part completes itself:
      // "4" in a day segment is the 4th, not the start of the 40-somethings
      const kind = order[seg];
      if (w === 2 && chunk.length === 1 && Number(chunk) * 10 > max[kind]) {
        out.push("0" + chunk);
        i = digits.length;
        return out.join(sep) + (seg < order.length - 1 ? sep : "");
      }
      out.push(chunk);
      return out.join(sep);
    }
    let n = Number(chunk);
    const kind = order[seg];
    if (kind !== "year") {
      if (n === 0) n = 1;
      else if (n > max[kind]) n = max[kind];
      out.push(String(n).padStart(2, "0"));
    } else {
      out.push(chunk);
    }
  }
  const done = i >= digits.length && out.length < order.length;
  return out.join(sep) + (done ? sep : "");
}

/** What an arrow key leaves in the input: the new text, its ISO, and the part to select. */
export interface DateStep {
  text: string;
  iso: string;
  start: number;
  end: number;
}

/** Which part the caret is in: the separators before it, counted. */
export function partAt(text: string, caret: number, parts: number): number {
  const before = text.slice(0, Math.max(0, caret)).replace(/\d/g, "").length;
  return Math.min(before, parts - 1);
}

/**
 * Steps the part of the date under the caret by `delta`, as a native date
 * input does with the arrow keys. Each part wraps within itself without
 * carrying — day 31 goes up to day 1 of the same month — and the day is
 * clamped when the month or year changes under it (31/01 → 28/02).
 *
 * An empty input fills with `todayIso` rather than stepping, so the first
 * press lands on today. Text that does not parse yet is left alone: stepping
 * would throw away what the user is typing.
 */
export function stepDate(text: string, caret: number, delta: number, todayIso: string): DateStep | null {
  const { order, sep, widths } = shape();
  const locate = (iso: string, idx: number): DateStep => {
    const out = formatDateLocal(iso);
    let start = 0;
    for (let i = 0; i < idx; i++) start += widths[i] + sep.length;
    return { text: out, iso, start, end: start + widths[idx] };
  };

  const current = text.trim();
  if (!current) {
    const today = parseDateLocal(todayIso);
    return today ? locate(today.iso, partAt("", caret, order.length)) : null;
  }
  const p = parseDateLocal(current);
  if (!p) return null;

  // a pasted ISO is year-month-day whatever the locale writes
  const own = ISO.test(current) ? (["year", "month", "day"] as const) : order;
  const kind = own[partAt(current, caret, own.length)];
  const wrap = (n: number, size: number) => ((((n - 1) % size) + size) % size) + 1;

  let { year, month, day } = p;
  if (kind === "day") day = wrap(day + delta, daysInMonth(year, month));
  else if (kind === "month") month = wrap(month + delta, 12);
  else year = Math.min(9999, Math.max(1000, year + delta));
  day = Math.min(day, daysInMonth(year, month));

  const iso = `${year}-${String(month).padStart(2, "0")}-${String(day).padStart(2, "0")}`;
  return locate(iso, order.indexOf(kind));
}

export interface CalendarDay {
  day: number;
  month: number;
  year: number;
  isCurrentMonth: boolean;
  iso: string;
}

/** Returns 35 or 42 days grid for a calendar month. */
export function getCalendarDays(year: number, month: number): CalendarDay[] {
  const firstDayOfWeek = new Date(year, month - 1, 1).getDay(); // 0 = Sun, 1 = Mon ...
  const daysInCurrent = daysInMonth(year, month);
  const prevMonth = month === 1 ? 12 : month - 1;
  const prevYear = month === 1 ? year - 1 : year;
  const daysInPrev = daysInMonth(prevYear, prevMonth);

  const nextMonth = month === 12 ? 1 : month + 1;
  const nextYear = month === 12 ? year + 1 : year;

  const days: CalendarDay[] = [];

  // Previous month overflow days
  for (let i = firstDayOfWeek - 1; i >= 0; i--) {
    const d = daysInPrev - i;
    const padD = String(d).padStart(2, "0");
    const padM = String(prevMonth).padStart(2, "0");
    days.push({
      day: d,
      month: prevMonth,
      year: prevYear,
      isCurrentMonth: false,
      iso: `${prevYear}-${padM}-${padD}`,
    });
  }

  // Current month days
  for (let d = 1; d <= daysInCurrent; d++) {
    const padD = String(d).padStart(2, "0");
    const padM = String(month).padStart(2, "0");
    days.push({
      day: d,
      month,
      year,
      isCurrentMonth: true,
      iso: `${year}-${padM}-${padD}`,
    });
  }

  // Next month overflow days (fill to multiple of 7, at least 35 or 42)
  const remaining = (7 - (days.length % 7)) % 7;
  const totalWanted = days.length + remaining < 35 ? 35 : days.length + remaining;
  const needNext = totalWanted - days.length;

  for (let d = 1; d <= needNext; d++) {
    const padD = String(d).padStart(2, "0");
    const padM = String(nextMonth).padStart(2, "0");
    days.push({
      day: d,
      month: nextMonth,
      year: nextYear,
      isCurrentMonth: false,
      iso: `${nextYear}-${padM}-${padD}`,
    });
  }

  return days;
}
