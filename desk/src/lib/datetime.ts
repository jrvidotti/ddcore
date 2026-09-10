// Civil-date helpers for the desk, kept in sync with `ddcore.utils` on the
// server (internal/js/prelude.js). The semantics are deliberately the same:
//
//   * a Date value is a *civil* date ("YYYY-MM-DD"), never an instant — so
//     `today()` is the day on the user's wall clock, not the UTC day;
//   * `addMonths` clamps the day to the last day of the target month
//     (31/01 + 1 mês = 28/02), exactly like `utils.addMonths` in the prelude.
//
// Datetime values, on the other hand, *are* instants: they travel as ISO
// strings in UTC and are shown/typed in the browser's local timezone.

const pad = (n: number) => String(n).padStart(2, "0");

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

/** The current civil date in the browser's timezone. */
export function today(now: Date = new Date()): string {
  return `${now.getFullYear()}-${pad(now.getMonth() + 1)}-${pad(now.getDate())}`;
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

/** Value for `<input type="datetime-local">`: the instant in local components. */
export function toDatetimeLocal(v: any): string {
  const d = parseDatetime(v);
  if (!d) return "";
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

/** Inverse of toDatetimeLocal: local wall-clock text back to an ISO instant. */
export function fromDatetimeLocal(s: string): string | null {
  if (!s) return null;
  // a datetime-local string carries no offset, so it is parsed as local time
  const d = new Date(s);
  return isNaN(d.getTime()) ? null : d.toISOString();
}
