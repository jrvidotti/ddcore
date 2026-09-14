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
  item: { route?: string; doctype?: string; report?: string },
): string {
  if (item.route) return item.route;
  if (item.doctype) {
    return `/app/${encodeURIComponent(workspaceName)}/${encodeURIComponent(item.doctype)}`;
  }
  if (item.report) {
    return `/app/${encodeURIComponent(workspaceName)}/report/${encodeURIComponent(item.report)}`;
  }
  return "";
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
    const directWs = workspaces.find((w) => w.name.toLowerCase() === part1.toLowerCase());
    if (directWs) return directWs;

    // 2. Is it /app/workspace/:name (legacy workspace route)?
    if (part1 === "workspace" && segments[2]) {
      const wsName = decodeURIComponent(segments[2]);
      const legacyWs = workspaces.find((w) => w.name.toLowerCase() === wsName.toLowerCase());
      if (legacyWs) return legacyWs;
    }

    // 3. Is it /app/report/:reportName (legacy report route)?
    if (part1 === "report" && segments[2]) {
      const repName = decodeURIComponent(segments[2]);
      const repWs = resolveWorkspaceForReport(repName, workspaces);
      if (repWs) {
        const found = workspaces.find((w) => w.name === repWs);
        if (found) return found;
      }
    }

    // 4. Is part1 a legacy un-prefixed DocType (e.g. /app/Contrato)?
    const dtWsName = resolveWorkspaceForDoctype(part1, workspaces, doctypes);
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
