import { goto } from "$app/navigation";
import { api, type DeskNotification } from "./api";
import { __ } from "./boot.svelte";
import { refreshNotifications } from "./notifications.svelte";
import { confirm } from "./ui.svelte";

/** Owns one open center; stale requests cannot overwrite a newer page or session. */
export class NotificationCenter {
  readonly limit = 20;
  offset = $state(0);
  filter = $state("all");
  rows = $state<DeskNotification[]>([]);
  total = $state(0);
  loading = $state(true);
  error = $state("");
  pending = $state("");
  private request = 0;
  private destroyed = false;

  async load(start: number, status: string) {
    const current = ++this.request;
    this.loading = true;
    this.error = "";
    // Clear documents immediately while their current access is revalidated.
    this.rows = [];
    try {
      const result = await api.notifications.list({ limit: this.limit, offset: start, read: status === "all" ? undefined : status === "read" });
      if (this.destroyed || current !== this.request) return;
      this.total = result.total;
      if (start > 0 && start >= this.total) {
        this.offset = Math.max(0, Math.ceil(this.total / this.limit) - 1) * this.limit;
        return;
      }
      this.rows = result.data;
    } catch (e) {
      if (!this.destroyed && current === this.request) { this.error = e instanceof Error ? e.message : String(e); this.total = 0; }
    } finally { if (!this.destroyed && current === this.request) this.loading = false; }
  }

  async toggle(row: DeskNotification) {
    this.pending = row.id;
    try {
      await api.notifications.setRead(row.id, !row.read);
      if (!this.destroyed) await refreshNotifications();
    } catch (e) {
      if (!this.destroyed) { this.rows = []; this.error = e instanceof Error ? e.message : String(e); }
    } finally { if (!this.destroyed) this.pending = ""; }
  }

  /** Opens the referenced document, offering to mark an unread notification as read first. */
  async open(row: DeskNotification) {
    if (!row.read && await confirm(__("Mark as read?"), __("Open document"))) {
      if (this.destroyed) return;
      await this.toggle(row);
      if (this.error) return;
    }
    if (!this.destroyed) await goto(`/app/${encodeURIComponent(row.reference_doctype)}/${encodeURIComponent(row.reference_id)}`);
  }

  destroy() { this.destroyed = true; this.request++; this.rows = []; }
}
