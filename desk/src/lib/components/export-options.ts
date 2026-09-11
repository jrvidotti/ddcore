// Building the URL of a full export.
//
// The desk's own CSV covers the page on screen. Exporting everything the
// filters match is a download the server streams, and the only thing the
// browser has to get right is the URL: the same filters the list is showing,
// so the file and the screen cannot disagree about what "the filtered set" is.
//
// This lives outside the component so it can be tested without one.

import { csvSep } from "../csv";

export type ExportFormat = "csv" | "ndjson";

export interface ExportOptions {
  doctype: string;
  format: ExportFormat;
  /** Include the child tables. NDJSON only — a CSV cannot nest them. */
  children?: boolean;
  filters?: unknown[];
  orFilters?: unknown[];
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
 * has, so it can only ever be a CSV, and only NDJSON can nest the child
 * tables. Offering the four valid combinations is what keeps the dialog from
 * accepting a format it would then have to ignore.
 */
export type ExportChoice = "page-csv" | "all-csv" | "all-ndjson" | "all-ndjson-children";

export const exportChoices: ExportChoice[] = ["page-csv", "all-csv", "all-ndjson", "all-ndjson-children"];

/**
 * What a choice asks the server for, or `null` for the page on screen — the
 * one export the browser writes itself.
 */
export function exportChoice(choice: string): { format: ExportFormat; children: boolean } | null {
  switch (choice) {
    case "all-csv": return { format: "csv", children: false };
    case "all-ndjson": return { format: "ndjson", children: false };
    case "all-ndjson-children": return { format: "ndjson", children: true };
    default: return null;
  }
}
