// Pure logic behind the global search palette: which DocTypes to offer as
// "go to" entries, how they merge with the server's document hits, and where
// each result leads.
import { resolveWorkspaceForDoctype, type WorkspaceItem } from "./sidebar-workspace";

export interface SearchHit {
  doctype: string;
  label: string;
  id: string;
  title: string;
}

export interface PaletteItem {
  kind: "doctype" | "document";
  doctype: string;
  label: string;
  /** The document id; empty for a DocType entry. */
  id: string;
  /** The line shown: the DocType label, or the document title. */
  title: string;
  href: string;
}

export const SEARCH_MIN_CHARS = 2;
const MAX_DOCTYPES = 5;

/** Lowercases and strips accents, as the server's search does. */
export function fold(s: string): string {
  return (s || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();
}

/** The workspace a result opens in: the one owning the DocType, else the remembered one, else "core". */
export function workspaceFor(
  doctype: string,
  workspaces: WorkspaceItem[],
  doctypes: Record<string, { app?: string }> | undefined,
  remembered: string,
): string {
  return resolveWorkspaceForDoctype(doctype, workspaces, doctypes) || remembered || workspaces[0]?.name || "core";
}

/** DocTypes whose label or name contain txt, prefix matches first. */
export function matchDoctypes(txt: string, doctypes: Record<string, { label?: string }>): [string, string][] {
  const needle = fold(txt.trim());
  if (!needle) return [];
  const rank = (s: string) => (s.startsWith(needle) ? 0 : 1);
  return Object.entries(doctypes || {})
    .map(([name, d]) => [name, d.label || name] as [string, string])
    .filter(([name, label]) => fold(label).includes(needle) || fold(name).includes(needle))
    .sort((a, b) => rank(fold(a[1])) - rank(fold(b[1])) || a[1].localeCompare(b[1]))
    .slice(0, MAX_DOCTYPES);
}

/** DocType entries first, then the documents in the order the server ranked them. */
export function buildItems(
  txt: string,
  hits: SearchHit[],
  ctx: { doctypes: Record<string, { label?: string; app?: string }>; workspaces: WorkspaceItem[]; remembered: string },
): PaletteItem[] {
  const ws = (dt: string) => encodeURIComponent(workspaceFor(dt, ctx.workspaces, ctx.doctypes, ctx.remembered));
  const items: PaletteItem[] = matchDoctypes(txt, ctx.doctypes).map(([doctype, label]) => ({
    kind: "doctype",
    doctype,
    label,
    id: "",
    title: label,
    href: `/app/${ws(doctype)}/${encodeURIComponent(doctype)}`,
  }));
  for (const h of hits) {
    items.push({
      kind: "document",
      doctype: h.doctype,
      label: h.label,
      id: h.id,
      title: h.title || h.id,
      href: `/app/${ws(h.doctype)}/${encodeURIComponent(h.doctype)}/${encodeURIComponent(h.id)}`,
    });
  }
  return items;
}

/** Moves the highlighted index by delta, wrapping around; -1 when there is nothing. */
export function moveIndex(current: number, delta: number, count: number): number {
  if (count <= 0) return -1;
  if (current < 0) return delta > 0 ? 0 : count - 1;
  return (((current + delta) % count) + count) % count;
}

/** Mod+K (Cmd on macOS, Ctrl elsewhere) opens the palette from anywhere. */
export function shouldOpenSearch(e: { key: string; ctrlKey?: boolean; metaKey?: boolean; altKey?: boolean; shiftKey?: boolean }): boolean {
  return Boolean((e.metaKey || e.ctrlKey) && !e.altKey && !e.shiftKey && e.key.toLowerCase() === "k");
}
