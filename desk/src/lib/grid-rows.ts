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

/** A grid filter's conditions as [field, operator, value] tuples. */
export function filterTuples(filters: any[][] | Record<string, any> | null | undefined): [string, string, any][] {
  if (!filters) return [];
  if (!Array.isArray(filters)) return Object.entries(filters).map(([k, v]) => [k, Array.isArray(v) ? "in" : "=", v]);
  return filters.map((t) => (t.length >= 3 ? [String(t[0]), String(t[1]).toLowerCase(), t[2]] : [String(t[0]), Array.isArray(t[1]) ? "in" : "=", t[1]]));
}

const isEmpty = (v: any) => v === null || v === undefined || v === "";
const asNumber = (v: any): number | null => (typeof v === "number" ? v : typeof v === "string" && v.trim() !== "" && Number.isFinite(Number(v)) ? Number(v) : null);

/** Compares two cell values: numbers as numbers, anything else as text (ISO dates sort as text). */
function compare(a: any, b: any): number {
  const na = asNumber(a), nb = asNumber(b);
  if (na !== null && nb !== null) return na - nb;
  return String(a) < String(b) ? -1 : String(a) > String(b) ? 1 : 0;
}

const likeRe = (pattern: string) =>
  new RegExp("^" + pattern.replace(/[.*+?^${}()|[\]\\]/g, "\\$&").replace(/%/g, ".*").replace(/_/g, ".") + "$", "is");

const listOf = (v: any): any[] => (Array.isArray(v) ? v : typeof v === "string" ? v.split(",").map((s) => s.trim()) : [v]);

/**
 * Whether a row meets one condition, the way the list's filters would read it
 * on the server: a Check compares as true/false (so `1`, `true` and a missing
 * value `0` agree), `like` takes % and _, `in` a list or "a, b".
 */
export function matchCondition(row: any, [field, op, value]: [string, string, any], col?: Pick<Field, "fieldtype">): boolean {
  let v = row?.[field];
  let want = value;
  if (col?.fieldtype === "Check") {
    v = !!v && v !== "0";
    if (op !== "in" && op !== "not in") want = !!want && want !== "0" && want !== "false";
  }
  switch (op) {
    case "=": return typeof v === "boolean" ? v === want : isEmpty(want) ? isEmpty(v) : !isEmpty(v) && compare(v, want) === 0;
    case "!=": return !matchCondition(row, [field, "=", value], col);
    case ">": return !isEmpty(v) && compare(v, want) > 0;
    case ">=": return !isEmpty(v) && compare(v, want) >= 0;
    case "<": return !isEmpty(v) && compare(v, want) < 0;
    case "<=": return !isEmpty(v) && compare(v, want) <= 0;
    case "like": return !isEmpty(v) && likeRe(String(want)).test(String(v));
    case "not like": return isEmpty(v) || !likeRe(String(want)).test(String(v));
    case "in": return listOf(want).some((w) => matchCondition(row, [field, "=", w], col));
    case "not in": return !listOf(want).some((w) => matchCondition(row, [field, "=", w], col));
    case "between": {
      const [lo, hi] = listOf(want);
      return !isEmpty(v) && compare(v, lo) >= 0 && compare(v, hi) <= 0;
    }
    case "is": return String(want).toLowerCase() === "not set" ? isEmpty(v) : !isEmpty(v);
    case "set": return !isEmpty(v);
    case "not set": return isEmpty(v);
  }
  return true;
}

/** The rows meeting every condition of every active filter (AND); the same array when none is active. */
export function filterRows<T>(rows: T[], active: (any[][] | Record<string, any>)[], columns: Pick<Field, "fieldname" | "fieldtype">[] = []): T[] {
  const conds = active.flatMap((f) => filterTuples(f));
  if (!conds.length) return rows;
  const colOf = (f: string) => columns.find((c) => c.fieldname === f);
  return rows.filter((r) => conds.every((c) => matchCondition(r, c, colOf(c[0]))));
}
