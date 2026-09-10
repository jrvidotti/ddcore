// SSE channel from /api/events: doc/list updates, reloads in dev, job results.
type Handler = (payload: any) => void;
const handlers = new Map<string, Set<Handler>>();
let source: EventSource | null = null;

export function connectEvents(onReload?: () => void) {
  if (source) return;
  source = new EventSource("/api/events");
  const names = ["doc_update", "list_update", "progress", "job_done", "reload", "reload_error", "hello"];
  for (const n of names) {
    source.addEventListener(n, (e: MessageEvent) => {
      let payload: any = null;
      try { payload = JSON.parse(e.data); } catch {}
      if (n === "reload") onReload?.();
      handlers.get(n)?.forEach((h) => h(payload));
    });
  }
  source.onerror = () => { source?.close(); source = null; setTimeout(() => connectEvents(onReload), 3000); };
}

export function subscribe(event: string, h: Handler): () => void {
  if (!handlers.has(event)) handlers.set(event, new Set());
  handlers.get(event)!.add(h);
  return () => handlers.get(event)?.delete(h);
}
