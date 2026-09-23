/**
 * Wraps an async task so a burst of calls runs it at most twice.
 *
 * A call while the task is idle starts it. Calls while it is running mark one
 * pending run, which starts once the current one ends — so the last caller
 * still sees fresh data, but a burst of N calls (one `list_update` per row of
 * a Data Import, say) never has more than one run in flight.
 */
export function coalesce(task: () => Promise<unknown>): () => Promise<void> {
  let running: Promise<void> | null = null;
  let pending = false;

  async function loop() {
    try {
      do {
        pending = false;
        try { await task(); } catch { /* the task reports its own errors */ }
      } while (pending);
    } finally {
      running = null;
    }
  }

  return () => {
    if (running) pending = true;
    else running = loop();
    return running;
  };
}
