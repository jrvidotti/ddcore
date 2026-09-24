/** Pure helpers for the To-Do page: its filters, its URL and its date windows. */
import { addDays } from "./datetime";
import type { PendingWorkOptions, ToDoDoc } from "./api";

export type TodoScope = "assigned_to_me" | "assigned_by_me";
export type TodoDue = "" | "overdue" | "today" | "week" | "none";
export type TodoView = "list" | "calendar" | "kanban";

export interface TodoFilters {
  q: string;
  priority: string;
  due: TodoDue;
  /** One due date (ISO), picked on the calendar; it takes the place of `due`. */
  date: string;
  /** The counterpart: who assigned my tasks, or whom I assigned them to. */
  user: string;
}

export interface TodoUrlState {
  scope: TodoScope;
  status: string;
  filters: TodoFilters;
  orderBy: string;
  page: number;
  pageSize: number;
  view: TodoView;
}

export const TODO_STATUSES = ["Open", "Closed", "Cancelled"] as const;
export const TODO_PRIORITIES = ["Low", "Medium", "High", "Urgent"] as const;
export const TODO_PAGE_SIZES = [20, 50, 100, 500];
const DUES: TodoDue[] = ["overdue", "today", "week", "none"];
const VIEWS: TodoView[] = ["list", "calendar", "kanban"];

export const emptyTodoFilters = (): TodoFilters => ({ q: "", priority: "", due: "", date: "", user: "" });
const ISO_DATE = /^\d{4}-\d{2}-\d{2}$/;

export function defaultTodoState(): TodoUrlState {
  return { scope: "assigned_to_me", status: "Open", filters: emptyTodoFilters(), orderBy: "", page: 1, pageSize: 20, view: "list" };
}

type DateWindow = Pick<PendingWorkOptions, "date_from" | "date_to" | "no_date" | "undated">;

/** The due-date window a "Due" choice stands for, relative to the site's today. */
export function dueRange(due: TodoDue, today: string): DateWindow {
  switch (due) {
    case "overdue": return { date_to: addDays(today, -1) };
    case "today": return { date_from: today, date_to: today };
    case "week": return { date_from: today, date_to: addDays(today, 6) };
    case "none": return { no_date: 1 };
    default: return {};
  }
}

/**
 * The window for one day. Tasks without a due date are shown on today, so
 * today's window keeps them too.
 */
export function dayRange(date: string, today: string): DateWindow {
  return date === today ? { date_from: date, date_to: date, undated: 1 } : { date_from: date, date_to: date };
}

/**
 * Intersects two windows: the later start and the earlier end win. Undated
 * tasks stay only where every window with bounds lets them in.
 */
export function intersectRange(a: DateWindow, b: DateWindow): DateWindow {
  const out: DateWindow = {};
  for (const w of [a, b]) {
    for (const [k, v] of Object.entries(w)) if (v !== undefined) (out as any)[k] = v;
  }
  if (a.date_from && b.date_from) out.date_from = a.date_from > b.date_from ? a.date_from : b.date_from;
  if (a.date_to && b.date_to) out.date_to = a.date_to < b.date_to ? a.date_to : b.date_to;
  const letsUndatedIn = (w: DateWindow) => w.undated === 1 || (!w.date_from && !w.date_to);
  if (out.undated && !(letsUndatedIn(a) && letsUndatedIn(b))) delete out.undated;
  return out;
}

/** The query /api/todo/pending gets for the filters; empty values are left out. */
export function todoQuery(filters: TodoFilters, orderBy: string, today: string, window: DateWindow = {}): PendingWorkOptions {
  const out: PendingWorkOptions = {};
  const q = filters.q.trim();
  if (q) out.q = q;
  if (filters.priority) out.priority = filters.priority;
  if (filters.user) out.user = filters.user;
  if (orderBy) out.order_by = orderBy;
  const range = filters.date ? dayRange(filters.date, today) : dueRange(filters.due, today);
  return { ...out, ...intersectRange(range, window) };
}

/** How many of the filters are set, for the badge on the Filters button. */
export function countTodoFilters(filters: TodoFilters): number {
  return [filters.q.trim(), filters.priority, filters.due || filters.date, filters.user].filter(Boolean).length;
}

export function hasTodoFilters(filters: TodoFilters): boolean {
  return countTodoFilters(filters) > 0;
}

/** Next sort for a column header: ascending first, then flips. */
export function nextTodoOrder(current: string, field: string): string {
  const [f, dir] = current.split(" ");
  return f === field && dir === "asc" ? `${field} desc` : `${field} asc`;
}

export function todoStateFromSearchParams(params: URLSearchParams): TodoUrlState {
  const d = defaultTodoState();
  const scope = params.get("scope");
  const status = params.get("status");
  const priority = params.get("priority") || "";
  const due = (params.get("due") || "") as TodoDue;
  const date = params.get("date") || "";
  const view = params.get("view") as TodoView | null;
  const page = Number(params.get("page"));
  const pageSize = Number(params.get("page_size"));
  return {
    scope: scope === "assigned_by_me" ? "assigned_by_me" : d.scope,
    status: status === "all" || (TODO_STATUSES as readonly string[]).includes(status || "") ? status! : d.status,
    filters: {
      q: params.get("q") || "",
      priority: (TODO_PRIORITIES as readonly string[]).includes(priority) ? priority : "",
      due: !ISO_DATE.test(date) && DUES.includes(due) ? due : "",
      date: ISO_DATE.test(date) ? date : "",
      user: params.get("user") || "",
    },
    orderBy: params.get("order_by") || "",
    page: Number.isInteger(page) && page > 0 ? page : 1,
    pageSize: TODO_PAGE_SIZES.includes(pageSize) ? pageSize : d.pageSize,
    view: view && VIEWS.includes(view) ? view : d.view,
  };
}

/** Only what differs from the defaults goes in the URL. */
export function todoStateToSearchParams(state: TodoUrlState): URLSearchParams {
  const d = defaultTodoState();
  const p = new URLSearchParams();
  if (state.scope !== d.scope) p.set("scope", state.scope);
  if (state.status !== d.status) p.set("status", state.status);
  if (state.filters.q.trim()) p.set("q", state.filters.q.trim());
  if (state.filters.priority) p.set("priority", state.filters.priority);
  if (state.filters.date) p.set("date", state.filters.date);
  else if (state.filters.due) p.set("due", state.filters.due);
  if (state.filters.user) p.set("user", state.filters.user);
  if (state.orderBy) p.set("order_by", state.orderBy);
  if (state.page > 1) p.set("page", String(state.page));
  if (state.pageSize !== d.pageSize) p.set("page_size", String(state.pageSize));
  if (state.view !== d.view) p.set("view", state.view);
  return p;
}

/** Returns the rows with one task's status changed, for an optimistic Kanban move. */
export function withTodoStatus(rows: ToDoDoc[], id: string, status: ToDoDoc["status"]): ToDoDoc[] {
  return rows.map((r) => (r.id === id ? { ...r, status } : r));
}
