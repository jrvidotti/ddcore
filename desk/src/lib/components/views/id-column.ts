import type { ListViewOptions } from "../../desk-sdk";
import type { DocTypeMeta, Field } from "../../meta";

/**
 * Whether the DocType's ids are random hashes: `idGeneration: { hash: true }`,
 * or no rule at all, which falls back to the same random id.
 */
export function hasHashID(doctype: DocTypeMeta): boolean {
  const g = doctype.idGeneration;
  return !g || !!g.hash || !(g.series || g.field || g.format || g.prompt);
}

/**
 * Whether the list shows its leading document-id column. The view decides
 * when it sets `idColumn`; otherwise the column is left out when the title
 * field is a column and is itself the id (the same value twice), and when the
 * id is a hash that means nothing to a reader — unless the list has no other
 * column to show.
 */
export function showIDColumn(doctype: DocTypeMeta, columns: Field[], settings: ListViewOptions = {}): boolean {
  if (settings.idColumn !== undefined) return settings.idColumn;
  if (hasHashID(doctype)) return columns.length === 0;
  return !columns.some((c) => c.fieldname === doctype.titleField) || doctype.idGeneration?.field !== doctype.titleField;
}
