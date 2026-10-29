// Turns the list's filter state into the API's filter tuples.

import { isTableType, type DocTypeMeta, type Field } from "$lib/meta";
import { calendarRangeFilters } from "./views/calendar-state";

/** An extra choice on a standard Select filter, registered with defineListView({ filterOptions }). */
export interface ListFilterOption {
  value: string;
  label: string;
  /** Applied in place of `[field, "=", value]` when this option is chosen. */
  filters: any[][];
}

export function buildListFilters(
  filters: Record<string, any>,
  options: Record<string, ListFilterOption[] | undefined> = {},
): any[][] {
  const out: any[][] = [];
  for (const [k, v] of Object.entries(filters)) {
    if (v === null || v === undefined || v === "") continue;
    const option = options[k]?.find((o) => o.value === v);
    if (option) out.push(...option.filters);
    else out.push([k, "=", v]);
  }
  return out;
}

/**
 * The Link a table's standard filter compares, and the filter path to it: a Table or Table
 * MultiSelect filters by the value its rows link to, so its child has to have exactly one Link.
 * Null for any other field.
 */
export function tableFilterLink(f: Field, children: Record<string, DocTypeMeta> = {}): { path: string; link: Field } | null {
  if (!isTableType(f.fieldtype) || typeof f.options !== "string") return null;
  const links = (children[f.options]?.fields || []).filter((cf) => cf.fieldtype === "Link" && cf.fieldname);
  return links.length === 1 ? { path: `${f.options}.${links[0].fieldname}`, link: links[0] } : null;
}

/** The API's tuple for a table's standard filter: a parent whose rows include the value. */
export function tableFilters(f: Field, children: Record<string, DocTypeMeta>, value: unknown): any[][] {
  const t = tableFilterLink(f, children);
  return t && value !== null && value !== undefined && value !== "" ? [[t.path, "=", value]] : [];
}

const ISO_DAY = /^\d{4}-\d{2}-\d{2}$/;
const day = (s: string | undefined) => (s && ISO_DAY.test(s) ? s : "");

/**
 * A range filter's value, registered with defineListView({ rangeFilters }): "from..to", where
 * either side may be empty. A bare day (an older link, a day picked on the calendar) is that day
 * alone, and anything that is not a YYYY-MM-DD day counts as no bound.
 */
export function parseRange(value: unknown): { from: string; to: string } {
  const s = typeof value === "string" ? value : "";
  if (!s.includes("..")) return { from: day(s), to: day(s) };
  const [from, to] = s.split("..", 2);
  return { from: day(from), to: day(to) };
}

/** The value `parseRange` reads back; "" when neither side is set. */
export function formatRange(from: string | null | undefined, to: string | null | undefined): string {
  return from || to ? `${from || ""}..${to || ""}` : "";
}

/** The API's tuples for a range filter; a Datetime's days run from their first to their last instant in the site's zone. */
export function rangeFilters(field: string, fields: Field[], value: unknown, tz?: string): any[][] {
  const { from, to } = parseRange(value);
  if (!from && !to) return [];
  const [lower, upper] = calendarRangeFilters(field, fields, from || to, to || from, tz);
  return [...(from ? [lower] : []), ...(to ? [upper] : [])];
}
