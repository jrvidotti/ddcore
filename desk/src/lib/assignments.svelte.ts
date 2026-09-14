import { api, type AssignArgs, type ToDoDoc } from "./api";
import { subscribe } from "./events";

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

  constructor(public doctype: string, public name: string) {}

  async load() {
    this.loading = true;
    this.error = "";
    try {
      this.rows = await api.assignments.forDoc(this.doctype, this.name);
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
      await api.assignments.assign(this.doctype, this.name, args);
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
      const idx = this.rows.findIndex((r) => r.name === todoName);
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
      this.rows = this.rows.filter((r) => r.name !== todoName);
      await refreshPendingCount();
    } catch (e) {
      this.error = e instanceof Error ? e.message : String(e);
      throw e;
    } finally {
      this.pending = "";
    }
  }
}

export class PendingWork {
  readonly limit = 20;
  offset = $state(0);
  status = $state("Open");
  scope = $state<"assigned_to_me" | "assigned_by_me">("assigned_to_me");
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
    this.rows = [];
    if (status !== undefined) this.status = status;
    if (scope !== undefined) this.scope = scope;
    this.offset = start;
    try {
      const result = await api.assignments.pending({
        limit: this.limit,
        offset: start,
        status: this.status === "all" ? undefined : this.status,
        scope: this.scope,
      });
      if (this.destroyed || current !== this.request) return;
      this.total = result.total;
      if (start > 0 && start >= this.total) {
        this.offset = Math.max(0, Math.ceil(this.total / this.limit) - 1) * this.limit;
        await this.load(this.offset);
        return;
      }
      this.rows = result.data;
    } catch (e) {
      if (!this.destroyed && current === this.request) {
        this.error = e instanceof Error ? e.message : String(e);
        this.total = 0;
      }
    } finally {
      if (!this.destroyed && current === this.request) {
        this.loading = false;
      }
    }
  }

  async complete(todoName: string) {
    this.pending = todoName;
    try {
      await api.assignments.complete(todoName);
      await refreshPendingCount();
      await this.load(this.offset);
    } catch (e) {
      if (!this.destroyed) this.error = e instanceof Error ? e.message : String(e);
    } finally {
      if (!this.destroyed) this.pending = "";
    }
  }

  async revoke(todoName: string) {
    this.pending = todoName;
    try {
      await api.assignments.revoke(todoName);
      await refreshPendingCount();
      await this.load(this.offset);
    } catch (e) {
      if (!this.destroyed) this.error = e instanceof Error ? e.message : String(e);
    } finally {
      if (!this.destroyed) this.pending = "";
    }
  }

  destroy() {
    this.destroyed = true;
    this.request++;
    this.rows = [];
  }
}
