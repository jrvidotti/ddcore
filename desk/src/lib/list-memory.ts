// The last URL each DocType's list was on in this tab — its filters, page and
// view — so a form's back button returns to the list as the user left it.
const lastHref = new Map<string, string>();

export function rememberListHref(doctype: string, href: string): void {
  lastHref.set(doctype, href);
}

export function lastListHref(doctype: string): string | undefined {
  return lastHref.get(doctype);
}

/** Only for tests. */
export function forgetListHrefs(): void {
  lastHref.clear();
}
