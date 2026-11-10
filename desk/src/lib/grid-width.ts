import type { Field } from "./meta";

// An inline grid lays its columns out by `columns` (table-layout: fixed): each column takes its share of the
// width the fixed columns (checkbox, #, actions) leave. Below the width this returns the table scrolls sideways
// instead of crushing its cells.

// what any column gets at least, on average: a Link input is not squeezed by the other columns
const AVERAGE_COLUMN = 88;
// A number does not wrap (the currency symbol is joined to it by a no-break space) and a right-aligned cell
// that cannot hold its value spills it over the next column, since CSS start-aligns a line that does not fit.
// So a numeric column gets the room of a typical value, padding included: "R$ 1.234.567" in a precision-0
// Currency, "R$ 123.456,78" with cents.
const NUMERIC_MIN: Record<string, number> = { Int: 64, Float: 72, Percent: 72, Duration: 88, Rating: 104 };
const currencyMin = (f: Field) => ((f.precision ?? 2) > 0 ? 124 : 104);

export function numericColumnMin(f: Field): number | undefined {
  return f.fieldtype === "Currency" ? currencyMin(f) : NUMERIC_MIN[f.fieldtype ?? ""];
}

/** The min-width, in px, of an inline grid with these columns and `fixed` px of fixed columns. */
export function inlineGridMinWidth(columns: Field[], fixed: number): number {
  const total = columns.reduce((s, c) => s + (c.columns || 2), 0);
  let width = columns.length * AVERAGE_COLUMN + fixed;
  for (const c of columns) {
    const min = numericColumnMin(c);
    // the column's share of what the fixed columns leave must hold `min`
    if (min) width = Math.max(width, Math.ceil((min * total) / (c.columns || 2)) + fixed);
  }
  return width;
}

// A `gridLayout: "fit"` grid (table-layout: auto) sizes each column to its widest value: a cell does not wrap, a
// `gridMaxWidth` column wraps past its cap, and the row buttons' column takes what is left over. An input has no
// width of its own to fit, so an editable column keeps the room a typical value needs.
const FIT_INPUT = 160;

/** The width, in px, a "fit" grid gives an editable column; a read-only one fits its values (undefined). */
export function fitColumnWidth(f: Field, editable: boolean): number | undefined {
  if (!editable) return undefined;
  return numericColumnMin(f) ?? Math.min(f.gridMaxWidth ?? FIT_INPUT, FIT_INPUT);
}

/** A header's label split before its last word, which a "fit" grid keeps on one line with the marks after it. */
export function splitLastWord(label: string): [string, string] {
  const i = label.trimEnd().lastIndexOf(" ");
  return i < 0 ? ["", label] : [label.slice(0, i + 1), label.slice(i + 1)];
}
