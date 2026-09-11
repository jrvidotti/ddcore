// The overdue routine and the per-status summary. The summary is shared by the
// report and by the workspace chart, so the two counts cannot drift apart.
import { whitelisted } from "@ddcore/sdk";
import type { Task } from "../.ddcore/types";

export const TASK_STATUSES = ["Open", "In progress", "Overdue", "Completed"] as const;
export type TaskStatus = (typeof TASK_STATUSES)[number];

export interface SummaryFilters {
  project?: string;
  assignee?: string;
  due_date_before?: string;
}

export interface SummaryRow {
  status: TaskStatus;
  count: number;
  share: number;
}

/**
 * Marks as Overdue every task whose due date has passed and that is not done.
 * It writes through the document rather than setValue, so the normal lifecycle
 * recalculates the project. One failure is logged and does not stop the run.
 */
export function markOverdue(): number {
  const pending = ddcore.db.getAll<{ name: string }>("Task", {
    filters: { status: ["in", ["Open", "In progress"]], due_date: ["<", ddcore.utils.today()] },
    fields: ["name"],
    limit: 10000,
  });

  let changed = 0;
  for (const row of pending) {
    try {
      const task = ddcore.getDoc<Task>("Task", row.name);
      task.status = "Overdue";
      task.save();
      changed++;
    } catch (e) {
      ddcore.log.error("Could not mark task " + row.name + " as overdue: " + String(e));
    }
  }
  return changed;
}

/** The scheduler's own rule, exposed for a manual run from the desk. */
export const markOverdueNow = whitelisted(() => ({ changed: markOverdue() }), {
  roles: ["Project Manager"],
});

/** One row per status — always all four — with its share of the total. */
export function summaryByStatus(filters: SummaryFilters = {}): SummaryRow[] {
  const conditions: Record<string, any> = {};
  if (filters.project) conditions.project = filters.project;
  if (filters.assignee) conditions.assignee = filters.assignee;
  if (filters.due_date_before) conditions.due_date = ["<=", filters.due_date_before];

  const tasks = ddcore.db.getList<{ status: string }>("Task", {
    filters: conditions,
    fields: ["status"],
    limit: 10000,
  });

  return TASK_STATUSES.map((status) => {
    const count = tasks.filter((t) => t.status === status).length;
    return {
      status,
      count,
      share: tasks.length === 0 ? 0 : ddcore.utils.roundTo((count * 100) / tasks.length, 2),
    };
  });
}

// There is no label map any more: a Select value *is* its own key, so a
// display label is `_(status)` at the point of rendering.
