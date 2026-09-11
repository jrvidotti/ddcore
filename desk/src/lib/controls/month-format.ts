// Month/year formatting, parsing and masking, with the order taken from the
// locale rather than fixed at "mm/yyyy". See date-format.ts for the reasoning.

import { monthNames, monthShape } from "../locale";

/** Abbreviated month names for the month picker's grid. */
export const monthLabelsShort = (): string[] => monthNames("short");

/** Full month names, for a title. */
export const monthLabelsFull = (): string[] => monthNames("long");

/** The hint for the input, e.g. "mm/yyyy" or "yyyy-mm". */
export const monthPlaceholder = (): string => monthShape().placeholder;

/** Formats an ISO date ("2026-09-01" or "2026-09") the way the locale writes it. */
export function formatMonth(v: any): string {
  if (!v) return "";
  const s = String(v).trim().slice(0, 10);
  const mIso = s.match(/^(\d{4})-(\d{1,2})(?:-\d{1,2})?$/);
  if (!mIso) return s;
  const { monthFirst, sep } = monthShape();
  const y = mIso[1];
  const m = mIso[2].padStart(2, "0");
  return monthFirst ? `${m}${sep}${y}` : `${y}${sep}${m}`;
}

export interface ParsedMonth {
  year: number;
  month: number;
  iso: string; // "YYYY-MM-01"
}

/** Parses the locale's own month order, or ISO, into year, month and ISO. */
export function parseMonth(text: string): ParsedMonth | null {
  if (!text) return null;
  const s = String(text).trim();

  const build = (y: number, m: number): ParsedMonth | null =>
    m >= 1 && m <= 12 && y >= 1000 && y <= 9999
      ? { year: y, month: m, iso: `${y}-${String(m).padStart(2, "0")}-01` }
      : null;

  // ISO is accepted everywhere: it is the wire format
  const mIso = s.match(/^(\d{4})-(\d{1,2})(?:-(\d{1,2}))?$/);
  if (mIso) return build(Number(mIso[1]), Number(mIso[2]));

  const { monthFirst, sep } = monthShape();
  const parts = s.split(sep).map((p) => p.trim());
  if (parts.length !== 2 || parts.some((p) => !/^\d{1,4}$/.test(p))) return null;
  const [a, b] = parts.map(Number);
  return monthFirst ? build(b, a) : build(a, b);
}

/** Formats typed digits into the locale's month order. */
export function maskMonthInput(raw: string): string {
  if (!raw) return "";
  const { monthFirst, sep } = monthShape();
  const digits = raw.replace(/\D/g, "").slice(0, 6);
  if (!digits) return "";

  if (!monthFirst) {
    // year first: "2026-09"
    const y = digits.slice(0, 4);
    if (digits.length <= 4) return y;
    let m = digits.slice(4, 6);
    const n = parseInt(m, 10);
    if (m.length === 2) m = n === 0 ? "01" : n > 12 ? "12" : String(n).padStart(2, "0");
    return `${y}${sep}${m}`;
  }

  if (digits.length === 1) {
    // months 2-9 cannot be preceded by a 1, so they complete themselves
    return digits >= "2" && digits <= "9" ? `0${digits}${sep}` : digits;
  }
  let m = digits.slice(0, 2);
  const y = digits.slice(2, 6);
  const n = parseInt(m, 10);
  if (n === 0) m = "01";
  else if (n > 12) m = "12";
  return y ? `${m}${sep}${y}` : `${m}${sep}`;
}
