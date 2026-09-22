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
