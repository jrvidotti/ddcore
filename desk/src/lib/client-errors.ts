// The last few things that went wrong in this tab: failed requests, uncaught
// errors and unhandled promise rejections. Feedback sends them as context, so
// a bug report carries what the person saw fail without them copying it.

export interface ClientError {
  /** ISO timestamp of when it happened. */
  at: string;
  kind: "request" | "error" | "rejection";
  message: string;
  method?: string;
  path?: string;
  status?: number;
  requestId?: string;
}

/** How many errors are kept; the oldest goes first. */
export const CLIENT_ERROR_LIMIT = 10;

const buffer: ClientError[] = [];
/** Errors already recorded where they were thrown, so a rejection does not record them again. */
const recorded = new WeakSet<object>();

/** Records `e`, and remembers `source` (the thrown error) so its rejection is not recorded twice. */
export function recordThrown(source: object, e: Omit<ClientError, "at">) {
  recorded.add(source);
  recordClientError(e);
}

/** A message is cut to this many characters: context, not a log dump. */
const MAX_MESSAGE = 500;

export function recordClientError(e: Omit<ClientError, "at"> & { at?: string }) {
  const entry: ClientError = { ...e, at: e.at || new Date().toISOString(), message: String(e.message ?? "").slice(0, MAX_MESSAGE) };
  for (const k of Object.keys(entry) as (keyof ClientError)[]) if (entry[k] === undefined) delete entry[k];
  buffer.push(entry);
  if (buffer.length > CLIENT_ERROR_LIMIT) buffer.splice(0, buffer.length - CLIENT_ERROR_LIMIT);
}

/** A copy of what is recorded, oldest first. */
export function recentClientErrors(): ClientError[] {
  return buffer.map((e) => ({ ...e }));
}

export function clearClientErrors() {
  buffer.length = 0;
}

/** A path without its query string: a search term or a token has no place in a report. */
export function requestPath(url: string): string {
  try {
    return new URL(url, "http://x").pathname;
  } catch {
    return String(url).split("?")[0];
  }
}

function reasonMessage(reason: unknown): string {
  if (reason instanceof Error) return reason.message || reason.name;
  if (typeof reason === "string") return reason;
  try { return JSON.stringify(reason) ?? String(reason); } catch { return String(reason); }
}

let installed = false;

/**
 * Listens for uncaught errors and unhandled rejections on `win`, once. A
 * rejection whose error was already recorded where it was thrown (a failed
 * request, see `recordThrown`) is not recorded twice.
 */
export function installClientErrorListeners(win: Window | undefined = typeof window === "undefined" ? undefined : window) {
  if (installed || !win) return;
  installed = true;
  win.addEventListener("error", (e: ErrorEvent) => {
    // a resource that failed to load fires a plain Event with no message
    if (!e || (!e.message && !e.error)) return;
    recordClientError({ kind: "error", message: e.message || reasonMessage(e.error) });
  });
  win.addEventListener("unhandledrejection", (e: PromiseRejectionEvent) => {
    const r: any = e?.reason;
    if (r && typeof r === "object" && recorded.has(r)) return;
    recordClientError({ kind: "rejection", message: reasonMessage(r) });
  });
}

/** For tests: forget that the listeners were installed. */
export function resetClientErrorListeners() {
  installed = false;
}
