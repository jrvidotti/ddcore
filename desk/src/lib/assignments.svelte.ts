import { api, type AssignArgs, type ToDoDoc } from "./api";
import { subscribe } from "./events";
import { today } from "./datetime";
import { registerTitles } from "./titles.svelte";
import { emptyTodoFilters, todoQuery, withTodoStatus, type TodoFilters } from "./todo-list";

export const pendingTasks = $state({ count: 0, revision: 0 });
let stopPending: (() => void) | null = null;

export async function refreshPendingCount() {
  try {
    const res = await api.assignments.pending({ limit: 1, status: "Open", scope: "assigned_to_me" });
    pendingTasks.count = res.total;
    pendingTasks.revision++;
  } catch {
    pendingTasks.count = 0;
  }
}

export function startPendingTasks() {
  if (stopPending) return;
  const refresh = () => { void refreshPendingCount(); };
  const visible = () => { if (typeof document !== "undefined" && document.visibilityState === "visible") refresh(); };
  const changed = subscribe("notifications_changed", refresh);
  const reconnected = subscribe("hello", refresh);
  if (typeof document !== "undefined") {
    document.addEventListener("visibilitychange", visible);
  }
  stopPending = () => {
    changed(); reconnected();
    if (typeof document !== "undefined") {
      document.removeEventListener("visibilitychange", visible);
    }
  };
  refresh();
}

export function stopPendingTasks() {
  stopPending?.();
  stopPending = null;
  pendingTasks.count = 0;
  pendingTasks.revision++;
}

export class DocAssignments {
  rows = $state<ToDoDoc[]>([]);
  loading = $state(false);
  error = $state("");
  pending = $state("");

  constructor(public doctype: string, public id: string) {}

  async load() {
    this.loading = true;
    this.error = "";
    try {
      this.rows = await api.assignments.forDoc(this.doctype, this.id);
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      this.rows = [];
    } finally {
      this.loading = false;
    }
  }

  async assign(args: AssignArgs) {
    this.error = "";
    try {
      await api.assignments.assign(this.doctype, this.id, args);
      await this.load();
      await refreshPendingCount();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      throw e;
    }
  }

  async complete(todoName: string) {
    this.pending = todoName;
    try {
      const updated = await api.assignments.complete(todoName);
      const idx = this.rows.findIndex((r) => r.id === todoName);
      if (idx !== -1) {
        this.rows[idx] = updated;
      }
      await refreshPendingCount();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      this.pending = "";
    }
  }

  async revoke(todoName: string) {
    this.pending = todoName;
    try {
      await api.assignments.revoke(todoName);
      this.rows = this.rows.filter((r) => r.id !== todoName);
      await refreshPendingCount();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      this.pending = "";
    }
  }
}

/** A window of rows loaded unpaged, as the Calendar and Kanban views need. */
export const TODO_WINDOW_LIMIT = 500;

export class PendingWork {
  limit = $state(20);
  offset = $state(0);
  status = $state("Open");
  scope = $state<"assigned_to_me" | "assigned_by_me">("assigned_to_me");
  filters = $state<TodoFilters>(emptyTodoFilters());
  orderBy = $state("");
  /**
   * Set by the Calendar and Kanban views: rows come unpaged, capped at
   * TODO_WINDOW_LIMIT, within these due-date bounds (the calendar's grid).
   */
  window = $state<{ date_from?: string; date_to?: string; allStatuses?: boolean } | null>(null);
  rows = $state<ToDoDoc[]>([]);
  total = $state(0);
  loading = $state(true);
  error = $state("");
  pending = $state("");
  private request = 0;
  private destroyed = false;

  async load(start = 0, status?: string, scope?: "assigned_to_me" | "assigned_by_me") {
    const current = ++this.request;
    this.loading = true;
    this.error = "";
    if (status !== undefined) this.status = status;
    if (scope !== undefined) this.scope = scope;
    this.offset = this.window ? 0 : start;
    try {
      const result = await api.assignments.pending({
        limit: this.window ? TODO_WINDOW_LIMIT : this.limit,
        offset: this.offset,
        // the Kanban's columns are the statuses: a status filter would empty all but one
        status: this.window?.allStatuses ? "all" : this.status,
        scope: this.scope,
        ...todoQuery(this.filters, this.orderBy, today(), { date_from: this.window?.date_from, date_to: this.window?.date_to }),
      });
      if (this.destroyed || current !== this.request) return;
      this.total = result.total;
      if (!this.window && start > 0 && start >= this.total) {
        this.offset = Math.max(0, Math.ceil(this.total / this.limit) - 1) * this.limit;
        await this.load(this.offset);
        return;
      }
      if (result.titles) registerTitles(result.titles);
      this.rows = result.data;
    } catch (e) {
      if (!this.destroyed && current === this.request) {
        this.error = e instanceof Error ? e.message : String(e);
        this.rows = [];
        this.total = 0;
      }
    } finally {
      if (!this.destroyed && current === this.request) {
        this.loading = false;
      }
    }
  }

  /**
   * Moves a task to a status through the assignment endpoints, which write
   * the timeline comment a plain update would not. The row changes at once
   * and goes back if the server refuses; the error is rethrown for the page
   * to show.
   */
  async setStatus(todoName: string, status: ToDoDoc["status"]) {
    const row = this.rows.find((r) => r.id === todoName);
    const previous = row?.status;
    if (row && previous === status) return;
    this.pending = todoName;
    if (row) this.rows = withTodoStatus(this.rows, todoName, status);
    try {
      if (status === "Closed") await api.assignments.complete(todoName);
      else if (status === "Cancelled") await api.assignments.revoke(todoName);
      else await api.assignments.reopen(todoName);
    } catch (e) {
      if (!this.destroyed && row && previous) this.rows = withTodoStatus(this.rows, todoName, previous);
      throw e;
    } finally {
      if (!this.destroyed) this.pending = "";
    }
    await refreshPendingCount();
    if (!this.destroyed) await this.load(this.offset);
  }

  complete(todoName: string) { return this.setStatus(todoName, "Closed"); }
  revoke(todoName: string) { return this.setStatus(todoName, "Cancelled"); }
  reopen(todoName: string) { return this.setStatus(todoName, "Open"); }

  destroy() {
    this.destroyed = true;
    this.request++;
    this.rows = [];
  }
}
