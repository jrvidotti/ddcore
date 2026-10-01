import { seg, resolveWorkspace, resolveReport, resolveDoctype } from "../routes";
export interface WorkspaceItem {
  name: string;
  label: string;
  icon?: string;
  app?: string;
  roles?: string[];
  sidebar?: Array<{
    label: string;
    doctype?: string;
    report?: string;
    route?: string;
    icon?: string;
  }>;
}

const WORKSPACE_KEY = "ddcore_workspace";

function defaultStore(): { getItem(k: string): string | null; setItem(k: string, v: string): void } | null {
  try {
    return (globalThis as any).localStorage ?? null;
  } catch {
    return null;
  }
}

export function rememberWorkspace(name: string, store = defaultStore()): void {
  try {
    if (store && name) {
      store.setItem(WORKSPACE_KEY, name);
    }
  } catch {
    /* private mode / SSR */
  }
}

export function getRememberedWorkspace(store = defaultStore()): string {
  try {
    if (store) {
      return store.getItem(WORKSPACE_KEY) || "";
    }
  } catch {
    /* private mode / SSR */
  }
  return "";
}

/**
 * Resolves which workspace owns a given DocType:
 * 1. Workspace that explicitly declares `doctype` in its sidebar.
 * 2. Workspace that belongs to the same app as the DocType.
 */
export function resolveWorkspaceForDoctype(
  doctype: string,
  workspaces: WorkspaceItem[],
  doctypesMeta?: Record<string, { app?: string }>,
): string | null {
  if (!doctype || !workspaces.length) return null;

  // 1. Direct match in workspace sidebar
  for (const ws of workspaces) {
    if (ws.sidebar?.some((item) => item.doctype === doctype)) {
      return ws.name;
    }
  }

  // 2. App-level match
  const dtApp = doctypesMeta?.[doctype]?.app;
  if (dtApp) {
    const ws = workspaces.find((w) => w.app === dtApp);
    if (ws) return ws.name;
  }

  return null;
}

/**
 * The workspace route a short `/app/<DocType>[/<id>]` URL redirects to; `rest`
 * is what follows the DocType (a record id, `new`). The query string and the
 * hash travel with it: a new form is prefilled from the query and a list reads
 * its filters from it.
 */
export function workspaceRedirect(workspace: string | null, doctype: string, rest: string[], url: { search: string; hash: string }): string {
  // a DocType no workspace owns stays on the short route, under its route name
  const names = workspace ? [seg(workspace), seg(doctype)] : [seg(doctype)];
  return `/app/${[...names, ...rest.map(encodeURIComponent)].join("/")}${url.search}${url.hash}`;
}

/**
 * Resolves which workspace owns a given report name.
 */
export function resolveWorkspaceForReport(
  reportName: string,
  workspaces: WorkspaceItem[],
): string | null {
  if (!reportName || !workspaces.length) return null;
  for (const ws of workspaces) {
    if (ws.sidebar?.some((item) => item.report === reportName)) {
      return ws.name;
    }
  }
  return null;
}

/**
 * Generates the prefixed URL for a sidebar item.
 */
export function workspaceItemHref(
  workspaceName: string,
  item: { label?: string; icon?: string; route?: string; doctype?: string; report?: string },
): string {
  if (item.route) return item.route;
  if (item.doctype) {
    return `/app/${seg(workspaceName)}/${seg(item.doctype)}`;
  }
  if (item.report) {
    return `/app/${seg(workspaceName)}/report/${seg(item.report)}`;
  }
  return "";
}

/** The reports the workspaces' sidebars list, keyed by name. */
function reportNames(workspaces: WorkspaceItem[]): Record<string, true> {
  const out: Record<string, true> = {};
  for (const ws of workspaces) for (const item of ws.sidebar || []) if (item.report) out[item.report] = true;
  return out;
}

/**
 * Resolves the active workspace from the current pathname, matching doctypes,
 * remembered workspace, or default fallback.
 */
export function resolveActiveWorkspace(params: {
  currentPath: string;
  workspaces: WorkspaceItem[];
  doctypes?: Record<string, { app?: string }>;
  remembered?: string;
}): WorkspaceItem | null {
  const { currentPath, workspaces, doctypes, remembered } = params;
  if (!workspaces || workspaces.length === 0) return null;

  // Normalize path parts: /app/:part1/:part2...
  const segments = currentPath.replace(/^\/+|\/+$/g, "").split("/");
  if (segments[0] === "app" && segments.length >= 2) {
    const part1 = decodeURIComponent(segments[1]);

    // 1. Does part1 match a known workspace name?
    const directName = resolveWorkspace(part1, { workspaces });
    const directWs = directName ? workspaces.find((w) => w.name === directName) : undefined;
    if (directWs) return directWs;

    // 2. Is it /app/workspace/:name (legacy workspace route)?
    if (part1 === "workspace" && segments[2]) {
      const wsName = resolveWorkspace(decodeURIComponent(segments[2]), { workspaces });
      const legacyWs = workspaces.find((w) => w.name === wsName);
      if (legacyWs) return legacyWs;
    }

    // 3. Is it /app/report/:reportName (legacy report route)?
    if (part1 === "report" && segments[2]) {
      const repName = resolveReport(decodeURIComponent(segments[2]), { reports: reportNames(workspaces) });
      const repWs = resolveWorkspaceForReport(repName, workspaces);
      if (repWs) {
        const found = workspaces.find((w) => w.name === repWs);
        if (found) return found;
      }
    }

    // 4. Is part1 a legacy un-prefixed DocType (e.g. /app/Contrato)?
    const dtWsName = resolveWorkspaceForDoctype(resolveDoctype(part1, { doctypes }), workspaces, doctypes);
    if (dtWsName) {
      const found = workspaces.find((w) => w.name === dtWsName);
      if (found) return found;
    }
  }

  // 5. Fall back to remembered workspace
  if (remembered) {
    const remWs = workspaces.find((w) => w.name.toLowerCase() === remembered.toLowerCase());
    if (remWs) return remWs;
  }

  // 6. Default to first workspace
  return workspaces[0];
}
