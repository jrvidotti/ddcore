import { api } from "./api";
import { liveCount } from "./live-count.svelte";

const unread = liveCount(() => api.notifications.count());
export const notifications = unread.state;
export const refreshNotifications = unread.refresh;
export const startNotifications = unread.start;
export const stopNotifications = unread.stop;
