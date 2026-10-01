/**
 * Names in a Desk path. A DocType, a workspace or a report goes into the URL
 * as its route name — the name without whitespace, `/app/TrainingClass` rather
 * than `/app/Training%20Class` — and is resolved back to the real name on the
 * way in. The name as it is, spaces and all, keeps resolving: it is what older
 * links, an app's `route:` and a mail already sent carry.
 */

/** What of the boot payload the resolvers read. */
export interface RouteNames {
  workspaces?: { name: string }[];
  doctypes?: Record<string, unknown>;
  virtuals?: Record<string, unknown>;
  reports?: Record<string, unknown>;
}

/** The form a DocType, workspace or report name takes in a Desk path: no whitespace. */
export function routeName(name: string): string {
  return String(name ?? "").replace(/\s+/g, "");
}

/** A name as a path segment. Idempotent, so a segment read back from the URL can pass through it. */
export function seg(name: string): string {
  return encodeURIComponent(routeName(name));
}

/** The real name a path segment stands for: the exact one first, then the one with that route name. */
export function resolveName(segment: string, names: Iterable<string>): string | null {
  const wanted = routeName(segment);
  let byRoute: string | null = null;
  for (const name of names) {
    if (name === segment) return name;
    if (byRoute === null && routeName(name) === wanted) byRoute = name;
  }
  return byRoute;
}

function knownDoctype(segment: string, names: RouteNames | null | undefined): string | null {
  return resolveName(segment, [...Object.keys(names?.doctypes || {}), ...Object.keys(names?.virtuals || {})]);
}

/**
 * The DocType a path segment names. One the boot payload does not list (a
 * child table, a DocType the user cannot read) comes back as it was, for the
 * server to answer.
 */
export function resolveDoctype(segment: string, names: RouteNames | null | undefined): string {
  return knownDoctype(segment, names) ?? segment;
}

/** The report a path segment names, or the segment itself when the boot payload does not list it. */
export function resolveReport(segment: string, names: RouteNames | null | undefined): string {
  return resolveName(segment, Object.keys(names?.reports || {})) ?? segment;
}

/** The workspace a path segment names, whatever its letter case; null when there is none. */
export function resolveWorkspace(segment: string, names: RouteNames | null | undefined): string | null {
  const all = (names?.workspaces || []).map((w) => w.name);
  const exact = resolveName(segment, all);
  if (exact) return exact;
  const wanted = routeName(segment).toLowerCase();
  return all.find((n) => routeName(n).toLowerCase() === wanted) ?? null;
}

// first segments under /app that are pages of their own, not names
const STATIC_PAGES = new Set(["notifications", "profile", "todo"]);

/**
 * The canonical spelling of an `/app` path — every name it carries as its
 * route name — or null when it already is canonical. Record ids, `new` and
 * `print` are left as they are, and so is the short `/app/<DocType>…` route:
 * its own redirect adds the workspace and writes the name canonically.
 */
export function canonicalAppPath(pathname: string, names: RouteNames | null | undefined): string | null {
  const raw = pathname.split("/").filter(Boolean);
  if (raw[0] !== "app" || raw.length < 2) return null;
  const dec = (s: string | undefined) => { try { return s === undefined ? undefined : decodeURIComponent(s); } catch { return s; } };
  const out = raw.slice(1);
  const part = out.map(dec) as string[];
  let changed = false;
  const put = (i: number, name: string | null) => {
    if (name === null || part[i] === routeName(name)) return;
    out[i] = seg(name);
    changed = true;
  };

  if (STATIC_PAGES.has(part[0])) return null;
  if (part[0] === "report") {
    if (part[1]) put(1, resolveName(part[1], Object.keys(names?.reports || {})));
  } else if (part[0] === "workspace") {
    if (part[1]) put(1, resolveWorkspace(part[1], names));
  } else {
    const ws = resolveWorkspace(part[0], names);
    if (!ws) return null;
    put(0, ws);
    if (part[1] === "report" && part[2]) put(2, resolveName(part[2], Object.keys(names?.reports || {})));
    else if (part[1]) put(1, knownDoctype(part[1], names));
  }
  return changed ? "/app/" + out.join("/") : null;
}
