// A modal owns the focus while it is open: it takes it when it appears, keeps
// Tab and Shift+Tab inside, and gives it back to whatever had it when it closes.
// Without this, a modal opened from a button leaves the focus on that button,
// and keys meant for the modal (Escape, Enter) go to the page behind it.

const TABBABLE = [
  "a[href]", "button:not([disabled])", "input:not([disabled]):not([type=hidden])",
  "select:not([disabled])", "textarea:not([disabled])", "[contenteditable]:not([contenteditable=false])",
  "[tabindex]:not([tabindex='-1'])",
].join(",");
const FIELD = "input:not([disabled]):not([type=hidden]), select:not([disabled]), textarea:not([disabled]), [contenteditable]:not([contenteditable=false])";

function visible(el: HTMLElement) {
  return !el.closest("[hidden], [inert]") && el.getAttribute("aria-hidden") !== "true";
}

function tabbables(node: HTMLElement) {
  return [...node.querySelectorAll<HTMLElement>(TABBABLE)].filter(visible);
}

/**
 * Where the focus lands when the modal opens: the first form field, so typing
 * starts right away and Enter submits; otherwise the modal itself, so Escape
 * reaches it without Enter pressing whichever button happens to come first.
 * A Link field opens its options list on focus, so when it comes first the
 * modal keeps the focus rather than pop the list open unasked.
 */
export function initialFocus(node: HTMLElement): HTMLElement {
  const field = [...node.querySelectorAll<HTMLElement>(FIELD)].find((el) => visible(el) && !(el as HTMLInputElement).readOnly);
  return field && !field.matches("[data-fieldtype=Link]") ? field : node;
}

export function focusTrap(node: HTMLElement) {
  const previous = document.activeElement as HTMLElement | null;
  if (!node.hasAttribute("tabindex")) node.setAttribute("tabindex", "-1");
  // a control with autofocus already took it
  if (!node.contains(document.activeElement)) initialFocus(node).focus({ preventScroll: true });

  function onkeydown(e: KeyboardEvent) {
    if (e.key !== "Tab") return;
    const els = tabbables(node);
    if (!els.length) { e.preventDefault(); node.focus(); return; }
    const first = els[0], last = els[els.length - 1];
    const at = document.activeElement;
    if (e.shiftKey && (at === first || at === node)) { e.preventDefault(); last.focus(); }
    else if (!e.shiftKey && (at === last || !node.contains(at))) { e.preventDefault(); first.focus(); }
  }
  node.addEventListener("keydown", onkeydown);

  return {
    destroy() {
      node.removeEventListener("keydown", onkeydown);
      if (previous?.isConnected && (!document.activeElement || document.activeElement === document.body || !document.activeElement.isConnected)) {
        previous.focus({ preventScroll: true });
      }
    },
  };
}
