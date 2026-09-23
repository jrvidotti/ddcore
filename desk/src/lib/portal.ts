// Portals (OPS-10): the client for /api/portal and the rules for where a
// person lands. A Website User never sees the desk; the server enforces it
// (every other /api route answers 403), and these helpers only keep the
// screen from sending them somewhere that would refuse them.
import { api } from "./api";
import type { Field } from "./meta";

export interface PortalPageMeta {
  portal: { name: string; slug: string; title: string; pages: { name: string; label: string; description?: string; kind: "list" | "record"; create?: boolean }[] };
  page: {
    name: string; label: string; description?: string; kind: "list" | "record";
    doctype: string; doctypeLabel: string; create: boolean; write: boolean;
    listFields: string[]; editable: string[] | null;
    actions: { label: string; page: string; new?: boolean }[];
    /** The workflow's state field, when the DocType has a workflow. */
    stateField: string; titleField?: string;
  };
  fields: Field[];
}

export interface PortalRows {
  rows: Record<string, any>[];
  titles: Record<string, Record<string, string>>;
  more: boolean;
}

const base = (portal: string, page: string) => `/api/portal/${encodeURIComponent(portal)}/${encodeURIComponent(page)}`;

export const portalApi = {
  page: (portal: string, page: string) => api.get<PortalPageMeta>(base(portal, page)),
  list: (portal: string, page: string, start = 0, limit = 20) => api.get<PortalRows>(base(portal, page) + "/list", { start, limit }),
  get: (portal: string, page: string, id?: string) =>
    api.get<Record<string, any>>(base(portal, page) + "/doc" + (id ? "/" + encodeURIComponent(id) : "")),
  defaults: (portal: string, page: string) => api.get<Record<string, any>>(base(portal, page) + "/new"),
  create: (portal: string, page: string, values: Record<string, any>) => api.post<Record<string, any>>(base(portal, page) + "/doc", values),
  update: (portal: string, page: string, id: string, values: Record<string, any>) =>
    api.put<Record<string, any>>(base(portal, page) + "/doc/" + encodeURIComponent(id), values),
  search: (portal: string, page: string, field: string, txt: string) =>
    api.get<{ id: string; title?: string }[]>(base(portal, page) + "/search/" + encodeURIComponent(field), { q: txt }),
};

export const isPortalPath = (path: string) => path === "/portal" || path.startsWith("/portal/");

/**
 * Where to go after signing in. `home` is the server's answer for this user
 * ("/portal" for a Website User); a portal user's own request is honoured only
 * inside the portal, anyone else's wherever it points.
 */
export function landing(home: string | undefined, redirect: string | null): string {
  if (home === "/portal") return redirect && isPortalPath(redirect) ? redirect : "/portal";
  return redirect || home || "/app";
}

/**
 * Where a signed-in user who opened `path` belongs: a Website User anywhere
 * but the portal or the sign-in screens is sent to the portal. Null means stay.
 */
export function portalRedirect(website: boolean | undefined, path: string): string | null {
  if (!website || isPortalPath(path) || path.startsWith("/login")) return null;
  return "/portal";
}

/**
 * The values a portal form sends: only the fields the page lets the user
 * type into, which is also all the server accepts.
 */
export function editableValues(doc: Record<string, any>, editable: string[] | null | undefined): Record<string, any> {
  const out: Record<string, any> = {};
  for (const f of editable || []) if (f in doc) out[f] = doc[f];
  return out;
}

/** The path of a portal page, a document on it, or its creation form. */
export function portalHref(portal: string, page: string, id?: string | "new"): string {
  const p = `/portal/${encodeURIComponent(portal)}/${encodeURIComponent(page)}`;
  return id ? `${p}/${id === "new" ? "new" : encodeURIComponent(id)}` : p;
}
