import { api } from "$lib/api";
import { stopNotifications } from "$lib/notifications.svelte";
import { stopPendingTasks } from "$lib/assignments.svelte";
import { disconnectEvents } from "$lib/events";

/** Ends the session, stops what listens on its behalf, and goes back to the sign-in page. */
export async function signOut() {
  await api.logout();
  stopNotifications();
  stopPendingTasks();
  disconnectEvents();
  location.href = "/login";
}
