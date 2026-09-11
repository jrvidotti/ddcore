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
