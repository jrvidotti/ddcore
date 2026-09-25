// Sorting and export of the rows a grid already holds in memory: a form's
// Table and a Report field (and the report page). Pure, so the Svelte grids
// only keep the state.

import { isNumericFieldtype, selectLabels, selectOptions, type Field } from "./meta";
import { csvSep, downloadCsv, toCsv } from "./csv";
import { downloadXlsx, type XlsxCell } from "./xlsx";

export interface GridSortState { field: string; order: "asc" | "desc" }

/** Resolves a Link's display title, when the grid knows it. */
export type TitleOf = (doctype: string, id: string) => string | undefined;

type Col = Pick<Field, "fieldname" | "fieldtype" | "options" | "label"> & Partial<Pick<Field, "optionLabels">>;

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });

/** The Link target of a column for a given row (a Dynamic Link reads it from a sibling). */
const linkTarget = (c: Col, row: any): string => (c.fieldtype === "Dynamic Link" ? row?.[c.options] : c.options) || "";

/** A Select value's display text. */
function selectLabel(c: Col, v: any): string {
  const i = selectOptions(c as Field).indexOf(String(v));
  return i >= 0 ? selectLabels(c as Field)[i] ?? String(v) : String(v);
}

/**
 * What a cell sorts by: numbers (and idx, Check) as numbers, dates and times as
 * their ISO text, a Link by its title, a Select by its label, anything else as
 * text. `null` means empty, which always sorts last.
 */
export function sortKey(row: any, c: Col | undefined, field: string, titleOf?: TitleOf): number | string | null {
  const v = row?.[field];
  if (v === null || v === undefined || v === "") return null;
  if (field === "idx" || (c && isNumericFieldtype(c.fieldtype))) {
    const n = Number(v);
    return Number.isFinite(n) ? n : null;
  }
  if (c?.fieldtype === "Check") return v ? 1 : 0;
  if (c && (c.fieldtype === "Link" || c.fieldtype === "Dynamic Link")) return titleOf?.(linkTarget(c, row), String(v)) || String(v);
  if (c?.fieldtype === "Select") return selectLabel(c, v);
  return typeof v === "object" ? JSON.stringify(v) : String(v);
}

/** Returns the rows in `sort` order, as a new array of the same row objects; stable. */
export function sortRows<T>(rows: T[], sort: GridSortState | null | undefined, columns: Col[], titleOf?: TitleOf): T[] {
  if (!sort?.field) return rows.slice();
  const c = columns.find((x) => x.fieldname === sort.field);
  const dir = sort.order === "desc" ? -1 : 1;
  const keyed = rows.map((row, i) => ({ row, i, k: sortKey(row, c, sort.field, titleOf) }));
  keyed.sort((a, b) => {
    if (a.k === null || b.k === null) return a.k === b.k ? a.i - b.i : a.k === null ? 1 : -1;
    const d = typeof a.k === "number" && typeof b.k === "number" ? a.k - b.k : collator.compare(String(a.k), String(b.k));
    return d ? d * dir : a.i - b.i;
  });
  return keyed.map((x) => x.row);
}

/**
 * The sort after clicking `field`'s header: ascending, then descending, then
 * back to the grid's default (`null`).
 */
export function nextSort(current: GridSortState | null, field: string, base: GridSortState | null = null): GridSortState | null {
  const effective = current ?? base;
  if (effective?.field !== field) return { field, order: "asc" };
  if (effective.order !== "desc") return { field, order: "desc" };
  // leaving a descending sort goes back to the default, unless the default is this very sort
  return base?.field === field && base.order === "desc" ? { field, order: "asc" } : null;
}

/** The value a cell exports: a Link's title, a Select's label, numbers and booleans as such. */
export function exportCell(row: any, c: Col, titleOf?: TitleOf): XlsxCell {
  const v = row?.[c.fieldname!];
  if (v === null || v === undefined) return "";
  if ((c.fieldtype === "Link" || c.fieldtype === "Dynamic Link") && v) return titleOf?.(linkTarget(c, row), String(v)) || String(v);
  if (c.fieldtype === "Select") return selectLabel(c, v);
  if (c.fieldtype === "Check") return !!v;
  if (isNumericFieldtype(c.fieldtype)) {
    const n = Number(v);
    return Number.isFinite(n) ? n : String(v);
  }
  return typeof v === "object" ? JSON.stringify(v) : v;
}

/** Header and cells for an export, in the order given. */
export function exportTable(rows: any[], columns: Col[], titleOf?: TitleOf): { header: string[]; cells: XlsxCell[][] } {
  return {
    header: columns.map((c) => c.label || c.fieldname || ""),
    cells: rows.map((r) => columns.map((c) => exportCell(r, c, titleOf))),
  };
}

/** A file name safe on every OS, without the extension. */
export function exportBaseName(...parts: (string | undefined)[]): string {
  return parts.filter(Boolean).join("-").replace(/[\\/:*?"<>|\u0000-\u001f]+/g, "_").slice(0, 120) || "export";
}

/** Downloads a table as CSV or XLSX. */
export function downloadTable(format: "csv" | "xlsx", base: string, header: string[], cells: XlsxCell[][], sheetName?: string): void {
  if (format === "xlsx") downloadXlsx(`${base}.xlsx`, header, cells, sheetName);
  else downloadCsv(`${base}.csv`, toCsv(header, cells, csvSep()));
}
