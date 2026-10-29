// Time formatting, parsing and masking for a text time input.
//
// A Time is a civil time on a 24-hour clock, "HH:MM:SS" on the wire, and the
// input writes it as "HH:MM" — the seconds only when they are not zero, so a
// value that carries them is never rounded away by opening the form.

import { partAt, type DateStep } from "./date-format";

const pad = (n: number) => String(n).padStart(2, "0");

const TIME = /^(\d{1,2}):(\d{1,2})(?::(\d{1,2})(?:\.\d+)?)?$/;

export interface ParsedTime {
  hour: number;
  minute: number;
  second: number;
  iso: string; // "HH:MM:SS"
}

/** Parses "HH:MM" or "HH:MM:SS" into its parts and the wire format. */
export function parseTime(text: string): ParsedTime | null {
  if (!text) return null;
  const m = String(text).trim().match(TIME);
  if (!m) return null;
  const [hour, minute, second] = [Number(m[1]), Number(m[2]), Number(m[3] ?? 0)];
  if (hour > 23 || minute > 59 || second > 59) return null;
  return { hour, minute, second, iso: `${pad(hour)}:${pad(minute)}:${pad(second)}` };
}

/** Formats a stored time ("08:00:00") as the input writes it ("08:00"). */
export function formatTime(v: any): string {
  if (!v) return "";
  const p = parseTime(String(v));
  if (!p) return String(v);
  return p.second ? p.iso : p.iso.slice(0, 5);
}

const MAX = [23, 59, 59];

/**
 * Formats typed digits as "HH:MM[:SS]", inserting the colon after the hour and
 * clamping each part. A lone digit that cannot start a two-digit part
 * completes itself: "8" is 08, not the start of the eighties.
 */
export function maskTimeInput(raw: string): string {
  if (!raw) return "";
  const digits = raw.replace(/\D/g, "").slice(0, 6);
  if (!digits) return "";

  const out: string[] = [];
  for (let seg = 0, i = 0; seg < 3 && i < digits.length; seg++, i += 2) {
    const chunk = digits.slice(i, i + 2);
    if (chunk.length === 1) {
      if (Number(chunk) * 10 > MAX[seg]) {
        out.push("0" + chunk);
        // the hour is followed by the minutes; the minutes need nothing after them
        return out.join(":") + (seg === 0 ? ":" : "");
      }
      out.push(chunk);
      return out.join(":");
    }
    out.push(pad(Math.min(Number(chunk), MAX[seg])));
  }
  return out.join(":") + (out.length === 1 ? ":" : "");
}

/**
 * Steps the hour, minute or second under the caret by `delta`, as a native
 * time input does with the arrow keys. Each part wraps within itself without
 * carrying. An empty input fills with `nowIso` rather than stepping; text that
 * does not parse yet is left alone.
 */
export function stepTime(text: string, caret: number, delta: number, nowIso: string): DateStep | null {
  const current = text.trim();
  const count = current ? current.split(":").length : 2;
  // the seconds stay on screen while they are stepped, even through zero
  const locate = (iso: string, idx: number): DateStep => {
    const start = idx * 3;
    return { text: count === 3 ? iso : iso.slice(0, 5), iso, start, end: start + 2 };
  };

  if (!current) {
    const now = parseTime(nowIso);
    return now ? locate(now.iso, 0) : null;
  }
  const p = parseTime(current);
  if (!p) return null;

  const parts = [p.hour, p.minute, p.second];
  const idx = partAt(current, caret, count);
  const size = idx === 0 ? 24 : 60;
  parts[idx] = (((parts[idx] + delta) % size) + size) % size;
  return locate(parts.map(pad).join(":"), idx);
}

/** The times of the day, `minutes` apart, for the picker's list. */
export function timeSlots(minutes: number): string[] {
  return Array.from({ length: Math.ceil(1440 / minutes) }, (_, i) => {
    const t = i * minutes;
    return `${pad(Math.floor(t / 60))}:${pad(t % 60)}:00`;
  });
}
