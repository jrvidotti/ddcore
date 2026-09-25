// Virtual DocTypes (DAT-07): a DocType with no table whose rows are the union
// of other DocTypes. A row's id — and the value a Link to one stores — is
// "<Source DocType>:<source id>". The desk never shows a form for the virtual
// DocType itself: a record URL for it is sent on to the source document, which
// is what makes every list view, Link cell and search hit open the right form
// without knowing anything about unions.
import { workspaceFor } from "./components/search-palette";
import type { WorkspaceItem } from "./components/sidebar-workspace";

export const VIRTUAL_SEP = ":";

/** Splits a virtual id at the first separator; null when it is not one. */
export function splitVirtualId(v: unknown): { doctype: string; id: string } | null {
  if (typeof v !== "string") return null;
  const i = v.indexOf(VIRTUAL_SEP);
  if (i <= 0 || i === v.length - 1) return null;
  return { doctype: v.slice(0, i), id: v.slice(i + 1) };
}

export interface RouteCtx {
  workspaces: WorkspaceItem[];
  doctypes: Record<string, { app?: string; label?: string }> | undefined;
  remembered: string;
}

/**
 * Where a record URL of a virtual DocType really leads: the list for "new"
 * (a virtual DocType has nothing to create), the source document otherwise,
 * opened in the workspace that owns the source. Null when the id names no
 * source, so the form can report the missing document as usual.
 */
export function virtualRedirect(doctype: string, id: string, sources: string[], ctx: RouteCtx): string | null {
  const ws = (dt: string) => encodeURIComponent(workspaceFor(dt, ctx.workspaces, ctx.doctypes, ctx.remembered));
  if (id === "new") return `/app/${ws(doctype)}/${encodeURIComponent(doctype)}`;
  const s = splitVirtualId(id);
  if (!s) return null;
  if (!sources.includes(s.doctype)) return null;
  return `/app/${ws(s.doctype)}/${encodeURIComponent(s.doctype)}/${encodeURIComponent(s.id)}`;
}

/** The source DocType's label for a virtual id, for a picker's subtitle. */
export function virtualSourceLabel(id: unknown, doctypes: Record<string, { label?: string }> | undefined): string {
  const s = splitVirtualId(id);
  if (!s) return "";
  return doctypes?.[s.doctype]?.label || s.doctype;
}

/**
 * A Link picker's subtitle for an option of a virtual DocType: the source's
 * label first, then the usual subtitle with the "<Source>:<id>" id shown as
 * the source's own id.
 */
export function virtualSubtitle(id: string, subtitle: string, doctypes: Record<string, { label?: string }> | undefined): string {
  const sourceId = splitVirtualId(id)?.id ?? id;
  const sub = subtitle ? subtitle.split(" · ").map((p) => (p === id ? sourceId : p)).join(" · ") : "";
  return [virtualSourceLabel(id, doctypes), sub].filter(Boolean).join(" · ");
}
