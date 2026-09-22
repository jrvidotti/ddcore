import type { ListViewOptions } from "../../desk-sdk";
import type { DocTypeMeta, Field } from "../../meta";

/**
 * Whether the list shows its leading document-name column. The view can turn
 * it off (`nameColumn: false`); otherwise it is left out only when the title
 * field is a column and is itself the name, which would show the same value twice.
 */
export function showNameColumn(doctype: DocTypeMeta, columns: Field[], settings: ListViewOptions = {}): boolean {
  if (settings.nameColumn === false) return false;
  return !columns.some((c) => c.fieldname === doctype.titleField) || doctype.naming?.field !== doctype.titleField;
}
