import type { Field } from "../meta";
import { isFieldHalfWidth } from "../meta";

/**
 * Converts the fields declared in each form column into visual rows. This
 * keeps matching field positions aligned even when one control has help text.
 */
export function fieldsByRow<T>(columns: T[][]): Array<Array<T | undefined>> {
  const visibleColumns = columns.map((column) => column.filter((field) => !(field && typeof field === "object" && "hidden" in field && (field as any).hidden === true)));
  const rowCount = Math.max(0, ...visibleColumns.map((column) => column.length));
  return Array.from({ length: rowCount }, (_, index) => visibleColumns.map((column) => column[index]));
}

/**
 * Packs fields in a column into visual lines (each line having either one full-width field
 * or up to two half-width fields).
 */
export function packColumnLines(fields: Field[], isVisible?: (f: Field) => boolean): Field[][] {
  const visible = fields.filter((f) => {
    if (!f) return false;
    if (typeof f === "object" && "hidden" in f && f.hidden === true) return false;
    if (isVisible && !isVisible(f)) return false;
    return true;
  });

  const lines: Field[][] = [];
  let currentLine: Field[] = [];

  for (const f of visible) {
    const half = isFieldHalfWidth(f);
    if (!half) {
      if (currentLine.length > 0) {
        lines.push(currentLine);
        currentLine = [];
      }
      lines.push([f]);
    } else {
      if (currentLine.length === 0) {
        currentLine.push(f);
      } else {
        currentLine.push(f);
        lines.push(currentLine);
        currentLine = [];
      }
    }
  }

  if (currentLine.length > 0) {
    lines.push(currentLine);
  }

  return lines;
}

/**
 * Arranges section columns into visual rows, where each row contains the fields
 * for each column on that visual line. This enables natural horizontal navigation
 * (left-to-right across the columns for each row).
 *
 * Returns: Array of rows, where each row is an array of column cells (each cell being Field[]).
 */
export function formRows(columns: Field[][], isVisible?: (f: Field) => boolean): Field[][][] {
  const colLines = columns.map((col) => packColumnLines(col, isVisible));
  const rowCount = Math.max(0, ...colLines.map((lines) => lines.length));
  return Array.from({ length: rowCount }, (_, rowIndex) =>
    colLines.map((lines) => lines[rowIndex] || [])
  );
}
