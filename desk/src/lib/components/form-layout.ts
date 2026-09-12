import type { Field } from "../meta";
import { FIELD_WIDTH_SLOTS, resolveFieldWidth } from "../meta";

/**
 * A form line is four quarters wide. Every width resolves to a number of these
 * slots, so a control keeps the same size whether or not its section was split
 * with a Column Break.
 */
export const LINE_SLOTS = 4;

/** Slots a column owns: a lone column takes the whole line, 2+ columns take half of it each. */
export function columnSlots(columnCount: number): number {
  return columnCount <= 1 ? LINE_SLOTS : LINE_SLOTS / 2;
}

/** Slots a field's cell takes inside a column of `capacity` slots. */
export function fieldSlots(field: Field, capacity: number): number {
  return Math.min(FIELD_WIDTH_SLOTS[resolveFieldWidth(field)], capacity);
}

/** Width class for a cell of `slots` inside a column of `capacity` ("" fills the column). */
export function cellWidthClass(slots: number, capacity: number): string {
  const fraction = slots / capacity;
  return fraction <= 0.25 ? "w-25" : fraction <= 0.5 ? "w-50" : "";
}

/** A packed cell: a field, or an alignment spacer when `field` is undefined. */
export interface LineCell {
  field?: Field;
  slots: number;
}

/**
 * Packs the fields of a column into visual lines of `capacity` slots.
 *
 * The fill is greedy but aligned: a field of `s` slots only starts at an offset
 * that is a multiple of `s`, so a half-line field never begins in the middle of
 * a quarter — `[1/4][1/2]` is padded to `[1/4][spacer][1/2]`.
 */
export function packColumnLines(fields: Field[], isVisible?: (f: Field) => boolean, capacity = LINE_SLOTS / 2): LineCell[][] {
  const visible = fields.filter((f) => {
    if (!f) return false;
    if (typeof f === "object" && "hidden" in f && f.hidden === true) return false;
    if (isVisible && !isVisible(f)) return false;
    return true;
  });

  const lines: LineCell[][] = [];
  let line: LineCell[] = [];
  let used = 0;
  const flush = () => {
    if (line.length > 0) lines.push(line);
    line = [];
    used = 0;
  };

  for (const f of visible) {
    const slots = fieldSlots(f, capacity);
    const pad = (slots - (used % slots)) % slots;
    if (used + pad + slots > capacity) {
      flush();
    } else if (pad > 0) {
      line.push({ slots: pad });
      used += pad;
    }
    line.push({ field: f, slots });
    used += slots;
    if (used >= capacity) flush();
  }
  flush();

  return lines;
}

/**
 * Arranges section columns into visual rows, where each row contains the fields
 * for each column on that visual line. This enables natural horizontal navigation
 * (left-to-right across the columns for each row).
 *
 * Returns: Array of rows, where each row is an array of column cells (each cell being LineCell[]).
 */
export function formRows(columns: Field[][], isVisible?: (f: Field) => boolean, capacity = LINE_SLOTS / 2): LineCell[][][] {
  const colLines = columns.map((col) => packColumnLines(col, isVisible, capacity));
  const rowCount = Math.max(0, ...colLines.map((lines) => lines.length));
  return Array.from({ length: rowCount }, (_, rowIndex) =>
    colLines.map((lines) => lines[rowIndex] || [])
  );
}
