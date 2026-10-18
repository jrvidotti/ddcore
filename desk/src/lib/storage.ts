// The browser's storage for the desk's per-browser preferences, shared by the
// modules that remember a choice (columns, filters, panels, drafts, ...).

/** The browser's localStorage, or null where there is none (SSR, private mode). */
export function defaultStore(): Storage | null {
  try {
    return (globalThis as any).localStorage ?? null;
  } catch {
    return null;
  }
}
