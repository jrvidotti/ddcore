/**
 * Converts the fields declared in each form column into visual rows. This
 * keeps matching field positions aligned even when one control has help text.
 */
export function fieldsByRow<T>(columns: T[][]): Array<Array<T | undefined>> {
  const visibleColumns = columns.map((column) => column.filter((field) => !(field && typeof field === "object" && "hidden" in field && field.hidden === true)));
  const rowCount = Math.max(0, ...visibleColumns.map((column) => column.length));
  return Array.from({ length: rowCount }, (_, index) => visibleColumns.map((column) => column[index]));
}
