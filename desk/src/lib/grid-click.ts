// Clicks on a grid's cells and rows (Table and Report fields): a script's
// onCellClick takes the whole cell, onRowClick whatever on the row takes no
// click of its own.

/** What takes a click of its own, so a click on it is not a click on the row. */
const OWN_CLICK = [
  "a", "button", "input", "select", "textarea", "label", "summary", "[contenteditable]",
  "[role=button]", "[role=checkbox]", "[role=link]", "[role=combobox]", "[role=listbox]", "[role=option]",
  "[tabindex]:not([tabindex='-1'])", "[data-own-click]",
].join(", ");

/**
 * A click on a cell's own area (its padding, or an empty value) goes to the
 * cell's button, so the whole cell is the target and not only its text. The
 * click stops there: it is the cell's, not the row's.
 */
export function forwardCellClick(e: MouseEvent) {
  if (e.target !== e.currentTarget) return;
  const button = (e.currentTarget as Element).querySelector<HTMLElement>(":scope > button.cell-click");
  if (!button) return;
  e.stopPropagation();
  button.click();
}

/**
 * Whether a click on a row (the handler's currentTarget) is for the row
 * itself: not on anything in it that takes a click of its own, and not the end
 * of a drag that selected text.
 */
export function isRowClick(e: MouseEvent): boolean {
  const row = e.currentTarget as Element | null;
  const target = e.target instanceof Element ? e.target : null;
  if (!row || !target) return false;
  const own = target.closest(OWN_CLICK);
  if (own && row.contains(own)) return false;
  const selection = typeof window !== "undefined" ? window.getSelection?.() : null;
  return !(selection && !selection.isCollapsed && selection.toString().trim());
}
