import { subscribe } from "./events";

/** A count that follows the server: refetched on a change, on reconnect and on returning to the tab; a stale response is dropped. */
export function liveCount(fetchCount: () => Promise<number>) {
  const state = $state({ count: 0, revision: 0 });
  let unsubscribe: (() => void) | null = null;
  let session = 0;
  let request = 0;

  async function refresh() {
    const mine = { session, request: ++request };
    const current = () => mine.session === session && mine.request === request;
    state.revision++;
    try {
      const count = await fetchCount();
      if (current()) state.count = count;
    } catch {
      // do not leave a stale count visible after access changes or a failed request
      if (current()) state.count = 0;
    }
  }

  function start() {
    if (unsubscribe) return;
    const reload = () => { void refresh(); };
    const visible = () => { if (document.visibilityState === "visible") reload(); };
    const changed = subscribe("notifications_changed", reload);
    const reconnected = subscribe("hello", reload);
    document.addEventListener("visibilitychange", visible);
    unsubscribe = () => {
      changed(); reconnected();
      document.removeEventListener("visibilitychange", visible);
    };
    reload();
  }

  function stop() {
    session++;
    unsubscribe?.(); unsubscribe = null;
    state.count = 0;
    state.revision++;
  }

  return { state, refresh, start, stop };
}
