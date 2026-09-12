import type { Field } from "../meta";
import { FIELD_WIDTH_SLOTS, resolveFieldWidth } from "../meta";

/**
 * A form line is four slots wide. Every width resolves to a number of them, so
 * a form is laid out by sizing its fields rather than by splitting it into
 * columns.
 */
export const LINE_SLOTS = 4;

/** Slots a field's cell takes on a line that can spend `capacity` of them. */
export function fieldSlots(field: Field, capacity: number): number {
  return Math.min(FIELD_WIDTH_SLOTS[resolveFieldWidth(field)], capacity);
}

/** Width class for a cell of `slots` on a line of `capacity` ("" fills the line). */
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
 * Packs the fields of a section into visual lines of `capacity` slots.
 *
 * The fill is greedy but aligned: a field of `s` slots only starts at an offset
 * that is a multiple of `s`, so a half-line field never begins in the middle of
 * a quarter — `[1/4][1/2]` is padded to `[1/4][spacer][1/2]`.
 */
export function packLines(fields: Field[], isVisible?: (f: Field) => boolean, capacity = LINE_SLOTS): LineCell[][] {
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
