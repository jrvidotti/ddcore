import type { ListViewOptions } from "../../desk-sdk";
import type { DocTypeMeta, Field } from "../../meta";

/**
 * Whether the list shows its leading document-id column. The view can turn
 * it off (`idColumn: false`); otherwise it is left out only when the title
 * field is a column and is itself the id, which would show the same value twice.
 */
export function showIDColumn(doctype: DocTypeMeta, columns: Field[], settings: ListViewOptions = {}): boolean {
  if (settings.idColumn === false) return false;
  return !columns.some((c) => c.fieldname === doctype.titleField) || doctype.idGeneration?.field !== doctype.titleField;
}
