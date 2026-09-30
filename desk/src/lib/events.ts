// SSE channel from /api/events: doc/list updates, reloads in dev, job results,
// and whatever an app sends with ddcore.publish.
type Handler = (payload: any) => void;
const handlers = new Map<string, Set<Handler>>();
let source: EventSource | null = null;
let reconnect: ReturnType<typeof setTimeout> | null = null;
let onReloadHook: (() => void) | undefined;
/** the names the current source dispatches: EventSource drops a named event nobody listens to */
let listening = new Set<string>();
const coreNames = ["doc_update", "list_update", "progress", "job_done", "reload", "reload_error", "hello", "notifications_changed", "maintenance"];

function listen(n: string) {
  if (!source || listening.has(n)) return;
  listening.add(n);
  source.addEventListener(n, (e: MessageEvent) => {
    let payload: any = null;
    try { payload = JSON.parse(e.data); } catch {}
    if (n === "reload") onReloadHook?.();
    handlers.get(n)?.forEach((h) => h(payload));
  });
}

export function connectEvents(onReload?: () => void) {
  if (source) return;
  onReloadHook = onReload;
  source = new EventSource("/api/events");
  listening = new Set();
  for (const n of [...coreNames, ...handlers.keys()]) listen(n);
  source.onerror = () => {
    source?.close(); source = null;
    reconnect = setTimeout(() => { reconnect = null; connectEvents(onReload); }, 3000);
  };
}

/** Stop both the live connection and a pending reconnect when the session ends. */
export function disconnectEvents() {
  if (reconnect !== null) clearTimeout(reconnect);
  reconnect = null;
  if (source) { source.onerror = null; source.close(); }
  source = null;
}

export function subscribe(event: string, h: Handler): () => void {
  if (!handlers.has(event)) handlers.set(event, new Set());
  handlers.get(event)!.add(h);
  listen(event);
  return () => unsubscribe(event, h);
}

export function unsubscribe(event: string, h: Handler) {
  handlers.get(event)?.delete(h);
}
