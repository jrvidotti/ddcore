// Civil-date helpers for the desk, kept in sync with `ddcore.utils` on the
// server (internal/js/prelude.js). The semantics are deliberately the same:
//
//   * a Date value is a *civil* date ("YYYY-MM-DD"), never an instant. It has
//     no timezone and is never converted;
//   * a Datetime *is* an instant. It travels as ISO in UTC and is shown and
//     typed in the **site's** timezone — not the browser's;
//   * a Time is a civil time and is never converted either.
//
// One timezone per site is what makes `due_date < today()` give the same
// answer on the client and on the server. Reading a Datetime in the browser's
// zone was the B18 bug: a form in America/Cuiaba showed, and saved back, a
// value four hours off.
//
// `addMonths` clamps the day to the last day of the target month (31/01 + 1
// month = 28/02), exactly like `utils.addMonths` in the prelude.

import { timezone } from "./locale";

const pad = (n: number) => String(n).padStart(2, "0");

/** The wall-clock parts of an instant in tz. */
function zoned(d: Date, tz: string): { y: number; m: number; d: number; h: number; min: number } {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone: tz, hour12: false,
    year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).formatToParts(d);
  const get = (t: string) => Number(parts.find((p) => p.type === t)?.value ?? 0);
  // "24" is how some engines spell midnight with hour12:false
  const h = get("hour") % 24;
  return { y: get("year"), m: get("month"), d: get("day"), h, min: get("minute") };
}

/** tz's offset from UTC, in minutes, at the instant d. */
function offsetAt(d: Date, tz: string): number {
  const z = zoned(d, tz);
  const asUTC = Date.UTC(z.y, z.m - 1, z.d, z.h, z.min);
  // seconds and milliseconds are not in the parts; keep them out of both sides
  const floored = Math.floor(d.getTime() / 60000) * 60000;
  return (asUTC - floored) / 60000;
}

/** Splits a civil date string ("YYYY-MM-DD", extra text ignored). */
function parts(d: string): [number, number, number] {
  const s = String(d).slice(0, 10);
  return [Number(s.slice(0, 4)), Number(s.slice(5, 7)) - 1, Number(s.slice(8, 10))];
}

/** Renders y/m/day as a civil date, normalising out-of-range month/day. */
function civil(y: number, m: number, day: number): string {
  const x = new Date(Date.UTC(y, m, day));
  return `${x.getUTCFullYear()}-${pad(x.getUTCMonth() + 1)}-${pad(x.getUTCDate())}`;
}

/** Number of days in the (possibly out-of-range) month m of year y. */
export function daysInMonth(y: number, m: number): number {
  return new Date(Date.UTC(y, m + 1, 0)).getUTCDate();
}

/**
 * The current civil date in the *site's* timezone — the same day the server
 * calls today, which is the whole point of having one site timezone.
 */
export function today(now: Date = new Date(), tz: string = timezone()): string {
  const z = zoned(now, tz);
  return `${z.y}-${pad(z.m)}-${pad(z.d)}`;
}

/** Adds n months, clamping the day to the end of the target month. */
export function addMonths(d: string, n: number): string {
  const [y, m, day] = parts(d);
  return civil(y, m + n, Math.min(day, daysInMonth(y, m + n)));
}

export function addDays(d: string, n: number): string {
  const [y, m, day] = parts(d);
  return civil(y, m, day + n);
}

export function monthStart(d?: string): string {
  const [y, m] = parts(d || today());
  return civil(y, m, 1);
}

export function monthEnd(d?: string): string {
  const [y, m] = parts(d || today());
  return civil(y, m, daysInMonth(y, m));
}

/** Parses a Datetime coming from the API into a Date (or null). */
export function parseDatetime(v: any): Date | null {
  if (v === null || v === undefined || v === "") return null;
  if (v instanceof Date) return isNaN(v.getTime()) ? null : v;
  // Postgres-style "2026-01-31 12:00:00" is not portable across engines
  const s = String(v).replace(" ", "T");
  const d = new Date(s);
  return isNaN(d.getTime()) ? null : d;
}

/** Value for `<input type="datetime-local">`: the instant in the site's zone. */
export function toDatetimeLocal(v: any, tz: string = timezone()): string {
  const d = parseDatetime(v);
  if (!d) return "";
  const z = zoned(d, tz);
  return `${z.y}-${pad(z.m)}-${pad(z.d)}T${pad(z.h)}:${pad(z.min)}`;
}

/**
 * Inverse of toDatetimeLocal: wall-clock text in the site's zone back to an
 * instant.
 *
 * The offset is found by probing, then applied and probed again: on a DST
 * boundary the offset at the guessed instant is not the offset at the real
 * one, and a single pass lands an hour out. Two passes converge everywhere a
 * civil time exists at all.
 */
export function fromDatetimeLocal(s: string, tz: string = timezone()): string | null {
  if (!s) return null;
  const m = String(s).match(/^(\d{4})-(\d{2})-(\d{2})[T ](\d{2}):(\d{2})/);
  if (!m) return null;
  const wall = Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3]), Number(m[4]), Number(m[5]));
  let guess = wall - offsetAt(new Date(wall), tz) * 60000;
  guess = wall - offsetAt(new Date(guess), tz) * 60000;
  const d = new Date(guess);
  return isNaN(d.getTime()) ? null : d.toISOString();
}
