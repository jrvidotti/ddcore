/**
 * A Table MultiSelect is child rows, one chosen value per row, held in the
 * child's single Link field. The control edits the values; these helpers turn
 * them back into rows, keeping the row (and its id) of every value that stays
 * so a save rewrites only what changed.
 */
import type { DocTypeMeta, Field } from "$lib/meta";

/** The child's one Link field. Mirrors meta.DocType.MultiSelectLinkField in Go. */
export function multiSelectLinkField(child: DocTypeMeta | undefined | null): Field | null {
  const links = (child?.fields || []).filter((f) => f.fieldtype === "Link");
  return links.length === 1 ? links[0] : null;
}

/** The chosen values, in order, skipping empty rows. */
export function multiSelectValues(rows: any, linkField: string): string[] {
  if (!Array.isArray(rows)) return [];
  return rows.map((r) => (typeof r === "string" ? r : r?.[linkField])).filter((v): v is string => !!v);
}

/** Rows with `value` appended; the same rows when it is already chosen or empty. */
export function withValue(rows: any, linkField: string, value: string, childDoctype: string): any[] {
  const current = Array.isArray(rows) ? rows : [];
  if (!value || multiSelectValues(current, linkField).includes(value)) return current;
  return renumber([...current, { doctype: childDoctype, [linkField]: value, __islocal: true }]);
}

/** Rows without `value`. */
export function withoutValue(rows: any, linkField: string, value: string): any[] {
  const current = Array.isArray(rows) ? rows : [];
  return renumber(current.filter((r) => (typeof r === "string" ? r : r?.[linkField]) !== value));
}

function renumber(rows: any[]): any[] {
  return rows.map((r, i) => (typeof r === "object" && r ? { ...r, idx: i + 1 } : r));
}
