// Whether the desk's two side bars are collapsed to an icon rail: the navigation
// sidebar and the form's right column. A per-browser choice, like the other
// `ddcore_*` view preferences.

import { defaultStore } from "./storage";

type Store = { getItem(k: string): string | null; setItem(k: string, v: string): void };

export const SIDEBAR_KEY = "ddcore_sidebar";
export const DOC_SIDEBAR_KEY = "ddcore_doc_sidebar";

/** The remembered choice for a bar; expanded when nothing valid is stored. */
export function panelCollapsed(key: string, store: Store | null = defaultStore()): boolean {
  try {
    return store?.getItem(key) === "collapsed";
  } catch {
    return false;
  }
}

export function rememberPanelCollapsed(key: string, collapsed: boolean, store: Store | null = defaultStore()): void {
  try {
    store?.setItem(key, collapsed ? "collapsed" : "expanded");
  } catch {
    /* private mode / SSR */
  }
}

export const panels = $state({
  sidebarCollapsed: panelCollapsed(SIDEBAR_KEY),
  docSidebarCollapsed: panelCollapsed(DOC_SIDEBAR_KEY),
});

export function setSidebarCollapsed(collapsed: boolean, store: Store | null = defaultStore()): void {
  panels.sidebarCollapsed = collapsed;
  rememberPanelCollapsed(SIDEBAR_KEY, collapsed, store);
}

export function setDocSidebarCollapsed(collapsed: boolean, store: Store | null = defaultStore()): void {
  panels.docSidebarCollapsed = collapsed;
  rememberPanelCollapsed(DOC_SIDEBAR_KEY, collapsed, store);
}
