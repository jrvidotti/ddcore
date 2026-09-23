import type { Field } from "./meta";
import { formatMonth } from "./controls/month-format.ts";
import { getLinkTitle } from "./titles.svelte";
import { __ } from "./boot.svelte";
import { currencyFmt, currencyPrecision, dateFmt, decimalSep, groupSep, currencySymbol, numberFmt, relativeFmt, roundingMode, timezone } from "./locale";
import { round } from "./round";
import { htmlToLine } from "./richtext";
import { formatDuration } from "./controls/duration-format";
import { ratingMax } from "./controls/rating-state";

export { formatMonth };

export const formatCurrency = (v: any, precision?: number) =>
  currencyFmt(precision !== undefined
    ? { minimumFractionDigits: precision, maximumFractionDigits: precision }
    : {},
  ).format(Number(v) || 0);

/** Rounds exactly the way the server is about to store the value. */
export const roundCurrency = (v: any, precision?: number) =>
  round(Number(v) || 0, precision ?? currencyPrecision(), roundingMode());

export const formatNumber = (v: any, precision?: number) =>
  numberFmt(precision !== undefined
    ? { minimumFractionDigits: precision, maximumFractionDigits: precision }
    : { maximumFractionDigits: 2 },
  ).format(Number(v) || 0);

/**
 * A Date is a civil date, with no timezone: 2026-03-01 is the first of March
 * everywhere. Formatting it as UTC is what keeps it from sliding a day in a
 * negative offset — the value is not an instant and must not be treated as one.
 */
export function formatDate(v: any): string {
  if (!v) return "";
  const s = String(v).slice(0, 10);
  const m = s.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!m) return String(v);
  const d = new Date(Date.UTC(Number(m[1]), Number(m[2]) - 1, Number(m[3])));
  return dateFmt({ day: "2-digit", month: "2-digit", year: "numeric", timeZone: "UTC" }).format(d);
}

/** A Datetime *is* an instant, and is shown in the site's timezone. */
export function formatDatetime(v: any): string {
  if (!v) return "";
  const d = new Date(String(v).replace(" ", "T"));
  if (isNaN(d.getTime())) return String(v);
  return dateFmt({
    day: "2-digit", month: "2-digit", year: "numeric",
    hour: "2-digit", minute: "2-digit", timeZone: timezone(),
  }).format(d);
}

/** A Duration's display flags, which never change what is stored. */
export const durationHides = (f: Partial<Field> | undefined, flag: string): boolean => {
  const o = f?.options as unknown;
  return Array.isArray(o) ? o.includes(flag) : o === flag;
};

export function formatValue(v: any, f?: Partial<Field>): string {
  if (v === null || v === undefined) return "";
  switch (f?.fieldtype) {
    case "Currency": return formatCurrency(v, f.precision || undefined);
    case "Percent": return formatNumber(v, f.precision ?? 2) + "%";
    case "Float": return formatNumber(v, f.precision);
    case "Int": return String(Math.round(Number(v)));
    case "Duration": return formatDuration(v, {
      hideDays: durationHides(f, "hideDays"), hideSeconds: durationHides(f, "hideSeconds"),
    });
    case "Rating": {
      const max = ratingMax(f.options);
      const n = Math.max(0, Math.min(max, Math.round(Number(v)) || 0));
      return "★".repeat(n) + "☆".repeat(max - n);
    }
    // a cell is one line: rich text loses its markup, Markdown and code their
    // line breaks
    // no sanitizer here: the cell is text, not markup, and stripping the tags
    // is what makes it text
    case "Text Editor": return htmlToLine(String(v));
    case "Markdown Editor":
    case "Code": return String(v).replace(/\s+/g, " ").trim();
    case "Check": return v ? "✓" : "";
    case "Date": return (f as any)?.options === "month" || (f as any)?.format === "mm/yyyy" ? formatMonth(v) : formatDate(v);
    case "Month": return formatMonth(v);
    case "Datetime": return formatDatetime(v);
    case "Link": return (f?.options ? getLinkTitle(f.options, v) : "") || String(v);
    case "Select": {
      // the value is canonical English and stays that way; what is shown is
      // its label, which the server filled in or which is the value's own key
      const opts = f.options;
      if (Array.isArray(opts) && Array.isArray(f.optionLabels)) {
        const i = opts.indexOf(v);
        if (i >= 0 && f.optionLabels[i]) return f.optionLabels[i];
      }
      return __(String(v));
    }
  }
  return String(v);
}

/**
 * A report summary card. Its datatype is a fieldtype, formatted like a cell of
 * that type; without one, a number is formatted as a number and anything else
 * is shown as the text it is.
 */
export function formatSummary(s: { value: any; datatype?: string }): string {
  if (s.datatype) return formatValue(s.value, { fieldtype: s.datatype } as Partial<Field>);
  if (typeof s.value === "number") return formatNumber(s.value);
  return s.value == null ? "" : String(s.value);
}

/**
 * Parses what the current locale prints: "1.234,56" in pt-BR, "1,234.56" in
 * en-US. The separators are derived, not assumed — reading "1,234" as 1.234
 * because the code knows only about commas is a silent, expensive error.
 */
export function parseNumber(s: string): number | null {
  if (s === null || s === undefined) return null;
  let t = String(s).trim();
  if (t === "") return null;
  const sym = currencySymbol();
  if (sym) t = t.split(sym).join("");
  t = t.replace(/[\s% ]/g, "");
  const dec = decimalSep();
  const grp = groupSep();
  if (grp) t = t.split(grp).join("");
  if (dec && dec !== ".") t = t.split(dec).join(".");
  if (t === "" || t === "-") return null;
  const n = Number(t);
  return isNaN(n) ? null : n;
}

const RELATIVE_STEPS: [number, Intl.RelativeTimeFormatUnit][] = [
  [60, "second"],
  [3600, "minute"],
  [86400, "hour"],
  [86400 * 30, "day"],
];

/** "3 minutes ago" / "há 3 minutos", from Intl rather than a table of strings. */
export function timeAgo(v: any): string {
  const d = new Date(String(v).replace(" ", "T"));
  if (isNaN(d.getTime())) return String(v ?? "");
  const diff = (Date.now() - d.getTime()) / 1000;
  if (Math.abs(diff) >= 86400 * 30) return formatDate(String(v).slice(0, 10));
  let divisor = 1;
  for (const [limit, unit] of RELATIVE_STEPS) {
    if (Math.abs(diff) < limit) return relativeFmt().format(-Math.round(diff / divisor), unit);
    divisor = limit;
  }
  return formatDate(String(v).slice(0, 10));
}

/**
 * Indicator colour for a status-like value, in precedence:
 *
 *  1. `optionColors` on the field — keyed by the canonical (English) value, so
 *     it is language-independent by construction. This is what an app should
 *     declare.
 *  2. a built-in catalogue of the canonical statuses the framework itself
 *     ships and documents.
 *  3. a deterministic hash into a small palette, so two different values never
 *     collide into looking like the same thing.
 *
 * What used to be here was a regex over Portuguese word stems. It read
 * "concluído" as green and had no idea what "completed" was, which is exactly
 * the bug this phase exists to remove: colour must never be decided by the
 * text a human happens to be reading.
 */
const CANONICAL_COLORS: Record<string, string> = {
  draft: "orange", pending: "orange", "pending approval": "orange", partial: "orange", waiting: "orange", "on hold": "orange",
  open: "blue", "in progress": "blue", active: "blue", rented: "blue", closed: "blue",
  completed: "green", done: "green", paid: "green", approved: "green", submitted: "green",
  available: "green", current: "green",
  overdue: "red", cancelled: "red", canceled: "red", rejected: "red", failed: "red",
  error: "red", expired: "red", inactive: "red", terminated: "red",
};

const PALETTE = ["blue", "green", "orange", "red", "purple", "gray"];

export function statusColor(v: string, field?: Partial<Field>): string {
  const raw = String(v ?? "");
  if (!raw) return "gray";
  if (field?.optionColors?.[raw]) return field.optionColors[raw];
  const known = CANONICAL_COLORS[raw.toLowerCase()];
  if (known) return known;
  let h = 0;
  for (let i = 0; i < raw.length; i++) h = (h * 31 + raw.charCodeAt(i)) >>> 0;
  return PALETTE[h % PALETTE.length];
}
