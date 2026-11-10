// Building the URL of a full export.
//
// The desk's own CSV covers the page on screen. Exporting everything the
// filters match is a download the server streams, and the only thing the
// browser has to get right is the URL: the same filters the list is showing,
// so the file and the screen cannot disagree about what "the filtered set" is.
//
// This lives outside the component so it can be tested without one.

import { csvSep } from "../csv";
import { wallClockMs } from "../datetime";
import { isNumericFieldtype, type Field } from "../meta";
import { excelSerial, type XlsxCell } from "../xlsx";

export type ExportFormat = "csv" | "ndjson" | "xlsx";

export interface ExportOptions {
  doctype: string;
  format: ExportFormat;
  /** Include the child tables. NDJSON only — a CSV or a workbook cannot nest them. */
  children?: boolean;
  filters?: unknown[];
  orFilters?: unknown[];
  /** The columns, in order (the list's visible ones); left out, every exportable column. */
  fields?: string[];
  /** The CSV column separator; defaults to the one the locale implies. */
  sep?: string;
}

/**
 * The URL of the export endpoint for these options.
 *
 * A plain GET, so the browser can navigate to it and download the file with
 * the session cookie it already has — no CSRF header to attach, and no blob
 * held in memory.
 */
export function exportUrl(o: ExportOptions): string {
  const q = new URLSearchParams();
  q.set("format", o.format);
  if (o.children && o.format === "ndjson") q.set("children", "1");
  if (o.filters?.length) q.set("filters", JSON.stringify(o.filters));
  if (o.orFilters?.length) q.set("or_filters", JSON.stringify(o.orFilters));
  if (o.fields?.length) q.set("fields", JSON.stringify(o.fields));
  if (o.format === "csv") q.set("sep", o.sep ?? csvSep());
  return `/api/export/${encodeURIComponent(o.doctype)}?${q}`;
}

/** Whether "include the child tables" applies to this format at all. */
export const supportsChildren = (format: ExportFormat): boolean => format === "ndjson";

/**
 * One choice in the export dialog.
 *
 * Scope and format are a single choice rather than two, because they are not
 * independent: the page on screen is a table of columns the browser already
 * has, so it can be a CSV or a workbook but never NDJSON, and only NDJSON can
 * nest the child tables. Offering the valid combinations is what keeps the
 * dialog from accepting a format it would then have to ignore.
 */
export type ExportChoice = "page-csv" | "page-xlsx" | "all-csv" | "all-xlsx" | "all-ndjson" | "all-ndjson-children";

export const exportChoices: ExportChoice[] = ["page-csv", "page-xlsx", "all-csv", "all-xlsx", "all-ndjson", "all-ndjson-children"];

/**
 * What a choice asks for: the page on screen, which the browser writes itself,
 * or everything the filters match, which the server streams.
 */
export function exportChoice(choice: string): { scope: "page" | "all"; format: ExportFormat; children: boolean } {
  switch (choice) {
    case "page-xlsx": return { scope: "page", format: "xlsx", children: false };
    case "all-csv": return { scope: "all", format: "csv", children: false };
    case "all-xlsx": return { scope: "all", format: "xlsx", children: false };
    case "all-ndjson": return { scope: "all", format: "ndjson", children: false };
    case "all-ndjson-children": return { scope: "all", format: "ndjson", children: true };
    default: return { scope: "page", format: "csv", children: false };
  }
}

/** Resolves a Link's display title, when the list has it. */
export type TitleOf = (doctype: string, id: string, titleField?: string) => string | undefined;

type PageCol = Pick<Field, "fieldname" | "fieldtype" | "options" | "label"> & Partial<Pick<Field, "linkTitleField">>;

const DAY_MS = 86400000;

/**
 * One cell of the page on screen, as the CSV holds it — a Link by its title,
 * anything else as the list loaded it — and, in a workbook, typed: numbers as
 * numbers, a Check as a boolean, a Date and a Time as real ones, a Datetime on
 * the site's clock. A Select keeps its stored value in both, and the id and
 * Modified are the same text as in the CSV.
 */
export function pageCell(row: any, c: PageCol | undefined, key: string, format: "csv" | "xlsx", titleOf?: TitleOf): XlsxCell {
  const v = row?.[key];
  if (c && (c.fieldtype === "Link" || c.fieldtype === "Dynamic Link") && v) {
    const target = c.fieldtype === "Dynamic Link" ? row?.[c.options] : c.options;
    return titleOf?.(target, String(v), c.fieldtype === "Link" ? c.linkTitleField : undefined) || v;
  }
  // modified stays its exact text, as in the CSV: Data Import compares it to
  // refuse a row changed since the export, and a day number would not match
  if (format === "csv" || !c || v === null || v === undefined || v === "") return v;
  const ft = c.fieldtype;
  if (ft === "Check") return !!v && v !== "0";
  if (isNumericFieldtype(ft)) {
    const n = Number(v);
    return Number.isFinite(n) ? n : v;
  }
  const s = String(v);
  if (ft === "Date" || ft === "Month") {
    const m = s.match(/^(\d{4})-(\d{2})-(\d{2})/);
    if (m) return { serial: excelSerial(Date.UTC(+m[1], +m[2] - 1, +m[3])), kind: "date" };
  }
  if (ft === "Datetime") {
    const d = new Date(s.replace(" ", "T"));
    if (!isNaN(d.getTime())) return { serial: excelSerial(wallClockMs(d)), kind: "datetime" };
  }
  if (ft === "Time") {
    const m = s.match(/^(\d{1,2}):(\d{2})(?::(\d{2}(?:\.\d+)?))?/);
    if (m) return { serial: ((+m[1] * 60 + +m[2]) * 60 + Number(m[3] ?? 0)) * 1000 / DAY_MS, kind: "time" };
  }
  return typeof v === "object" ? JSON.stringify(v) : v;
}

/**
 * The page on screen as a table: the id, always and first, then the columns
 * on screen in their order, then Modified when it is shown.
 */
export function pageTable(
  rows: any[], columns: PageCol[],
  o: { idLabel: string; modifiedLabel?: string; format: "csv" | "xlsx"; titleOf?: TitleOf },
): { header: string[]; cells: XlsxCell[][] } {
  const keys = ["id", ...columns.map((c) => c.fieldname!), ...(o.modifiedLabel ? ["modified"] : [])];
  const cols: (PageCol | undefined)[] = [undefined, ...columns, ...(o.modifiedLabel ? [undefined] : [])];
  return {
    header: [o.idLabel, ...columns.map((c) => c.label || c.fieldname || ""), ...(o.modifiedLabel ? [o.modifiedLabel] : [])],
    cells: rows.map((r) => keys.map((k, i) => pageCell(r, cols[i], k, o.format, o.titleOf))),
  };
}
