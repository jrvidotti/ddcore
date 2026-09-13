import { api } from "./api";
import { subscribe } from "./events";

export const notifications = $state({ unread: 0, revision: 0 });
let stop: (() => void) | null = null;
let generation = 0;
let countRequest = 0;

export async function refreshNotifications() {
  const session = generation;
  const request = ++countRequest;
  notifications.revision++;
  try {
    const count = await api.notifications.count();
    if (session === generation && request === countRequest) notifications.unread = count;
  } catch {
    // Do not leave a stale count visible after access changes or a failed request.
    if (session === generation && request === countRequest) notifications.unread = 0;
  }
}

export function startNotifications() {
  if (stop) return;
  const refresh = () => { void refreshNotifications(); };
  const visible = () => { if (document.visibilityState === "visible") refresh(); };
  const changed = subscribe("notifications_changed", refresh);
  const reconnected = subscribe("hello", refresh);
  document.addEventListener("visibilitychange", visible);
  stop = () => {
    changed(); reconnected();
    document.removeEventListener("visibilitychange", visible);
  };
  refresh();
}

export function stopNotifications() {
  generation++;
  stop?.(); stop = null;
  notifications.unread = 0;
  notifications.revision++;
}
