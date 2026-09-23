/**
 * A Duration is whole seconds. What the user sees is `1d 2h 30m`, and what is
 * stored is the number — the same formatting the server prints, so a list, a
 * form and a PDF agree.
 */
export interface DurationOptions {
  hideDays?: boolean;
  hideSeconds?: boolean;
}

export interface DurationParts {
  days: number;
  hours: number;
  minutes: number;
  seconds: number;
}

/** Splits seconds into the boxes the control shows. */
export function splitDuration(secs: number, o: DurationOptions = {}): DurationParts {
  let s = Math.max(0, Math.floor(Number(secs) || 0));
  if (o.hideSeconds) s -= s % 60;
  let days = 0;
  if (!o.hideDays) {
    days = Math.floor(s / 86400);
    s %= 86400;
  }
  const hours = Math.floor(s / 3600);
  s %= 3600;
  return { days, hours, minutes: Math.floor(s / 60), seconds: s % 60 };
}

export const joinDuration = (p: Partial<DurationParts>): number =>
  Math.max(0, Math.floor(
    (Number(p.days) || 0) * 86400 + (Number(p.hours) || 0) * 3600 +
    (Number(p.minutes) || 0) * 60 + (Number(p.seconds) || 0),
  ));

/** The same text the server's FormatDuration writes. */
export function formatDuration(secs: any, o: DurationOptions = {}): string {
  if (secs === null || secs === undefined || secs === "") return "";
  const p = splitDuration(Number(secs), o);
  const parts: string[] = [];
  if (p.days) parts.push(`${p.days}d`);
  if (p.hours) parts.push(`${p.hours}h`);
  if (p.minutes) parts.push(`${p.minutes}m`);
  if (p.seconds && !o.hideSeconds) parts.push(`${p.seconds}s`);
  if (!parts.length) return o.hideSeconds ? "0m" : "0s";
  return parts.join(" ");
}

/**
 * Reads what a person types in a grid cell: `1d 2h 30m`, `90m`, `1:30` (hours
 * and minutes) or a bare number of seconds. Anything else is `null`, which the
 * control shows as unset rather than as zero.
 */
export function parseDuration(text: string, o: DurationOptions = {}): number | null {
  const s = String(text ?? "").trim().toLowerCase();
  if (!s) return null;
  if (/^\d+$/.test(s)) return joinDuration(o.hideSeconds ? { minutes: Number(s) } : { seconds: Number(s) });
  const clock = s.match(/^(\d+):([0-5]?\d)(?::([0-5]?\d))?$/);
  if (clock) {
    return joinDuration({ hours: Number(clock[1]), minutes: Number(clock[2]), seconds: Number(clock[3] || 0) });
  }
  const units = [...s.matchAll(/(\d+(?:[.,]\d+)?)\s*([dhms])/g)];
  if (!units.length) return null;
  const by: Record<string, number> = { d: 86400, h: 3600, m: 60, s: 1 };
  let total = 0;
  for (const [, n, u] of units) total += Number(String(n).replace(",", ".")) * by[u];
  return Math.max(0, Math.floor(total));
}

export type DurationUnit = keyof DurationParts;
export type DurationLabels = Record<DurationUnit, string>;

const UNIT_SECS: Record<DurationUnit, number> = { days: 86400, hours: 3600, minutes: 60, seconds: 1 };
// what a unit tops out at when a bigger unit sits before it; the first one has no top
const UNIT_CAP: Record<DurationUnit, number> = { days: Infinity, hours: 23, minutes: 59, seconds: 59 };

/** The units the control shows, biggest first. */
export function durationUnits(o: DurationOptions = {}): DurationUnit[] {
  const units: DurationUnit[] = ["days", "hours", "minutes", "seconds"];
  return units.filter((u) => !(u === "days" && o.hideDays) && !(u === "seconds" && o.hideSeconds));
}

/** One unit's digits in the control's text: `start`..`end` is what a key edits. */
export interface DurationSegment {
  unit: DurationUnit;
  start: number;
  end: number;
}

/**
 * Writes the single-input form, `01d 02h 30m 00s`, and where each unit's
 * digits are. The labels are the translated ones, so the ranges follow them.
 */
export function durationSegments(secs: any, o: DurationOptions, labels: DurationLabels): { text: string; segs: DurationSegment[] } {
  const parts = splitDuration(Number(secs) || 0, o);
  let text = "";
  const segs: DurationSegment[] = [];
  for (const unit of durationUnits(o)) {
    if (text) text += " ";
    const start = text.length;
    text += String(parts[unit]).padStart(2, "0");
    segs.push({ unit, start, end: text.length });
    text += labels[unit];
  }
  return { text, segs };
}

/** The segment the caret is in; right after a unit's digits still counts as that unit. */
export function segmentAt(segs: DurationSegment[], caret: number): number {
  let i = 0;
  while (i + 1 < segs.length && segs[i + 1].start <= caret) i++;
  return i;
}

/** An arrow on a unit: the total moves by that unit, carrying into the next, never below zero. */
export const stepDuration = (secs: any, unit: DurationUnit, delta: number): number =>
  Math.max(0, (Math.floor(Number(secs)) || 0) + UNIT_SECS[unit] * delta);

/**
 * A digit typed on a unit, as a native time input takes it: the first one
 * replaces the unit, the second appends, and a unit after the first one moves
 * on once it is full — or once no second digit could keep it under its cap.
 */
export function typeDigit(
  secs: any, o: DurationOptions, unit: DurationUnit, buffer: string, digit: string,
): { secs: number; buffer: string; advance: boolean } {
  const lead = durationUnits(o)[0] === unit;
  const cap = lead ? Infinity : UNIT_CAP[unit];
  const typed = (buffer + digit).slice(0, lead ? 6 : 2);
  const n = Math.min(Number(typed), cap);
  const parts = splitDuration(Number(secs) || 0, o);
  parts[unit] = n;
  const advance = !lead && (typed.length >= 2 || n * 10 > cap);
  return { secs: joinDuration(parts), buffer: advance ? "" : typed, advance };
}
