import { describe, expect, it } from "vitest";
import {
  countTodoFilters, defaultTodoState, dueRange, emptyTodoFilters, intersectRange, nextTodoOrder, todoQuery,
  todoStateFromSearchParams, todoStateToSearchParams, withTodoStatus,
} from "./todo-list";
import type { ToDoDoc } from "./api";

describe("dueRange", () => {
  const today = "2026-09-24";
  it("maps each choice to a due-date window", () => {
    expect(dueRange("", today)).toEqual({});
    expect(dueRange("overdue", today)).toEqual({ date_to: "2026-09-23" });
    expect(dueRange("today", today)).toEqual({ date_from: today, date_to: today });
    expect(dueRange("week", today)).toEqual({ date_from: today, date_to: "2026-09-30" });
    expect(dueRange("none", today)).toEqual({ no_date: 1 });
  });
});

describe("intersectRange", () => {
  it("keeps the later start and the earlier end", () => {
    expect(intersectRange({ date_from: "2026-09-24", date_to: "2026-09-30" }, { date_from: "2026-08-30", date_to: "2026-10-10" }))
      .toEqual({ date_from: "2026-09-24", date_to: "2026-09-30" });
    expect(intersectRange({ date_to: "2026-09-23" }, { date_from: "2026-08-30", date_to: "2026-10-10" }))
      .toEqual({ date_from: "2026-08-30", date_to: "2026-09-23" });
  });
});

describe("todoQuery", () => {
  it("leaves empty filters out", () => {
    expect(todoQuery(emptyTodoFilters(), "", "2026-09-24")).toEqual({});
  });
  it("sends the set filters, the order and the window", () => {
    expect(todoQuery({ q: " leite ", priority: "High", due: "week", user: "ana@x.com" }, "date asc", "2026-09-24", { date_from: "2026-09-01", date_to: "2026-09-27" }))
      .toEqual({ q: "leite", priority: "High", user: "ana@x.com", order_by: "date asc", date_from: "2026-09-24", date_to: "2026-09-27" });
  });
});

describe("nextTodoOrder", () => {
  it("sorts ascending first, then flips", () => {
    expect(nextTodoOrder("", "date")).toBe("date asc");
    expect(nextTodoOrder("date asc", "date")).toBe("date desc");
    expect(nextTodoOrder("date desc", "date")).toBe("date asc");
    expect(nextTodoOrder("date desc", "priority")).toBe("priority asc");
  });
});

describe("URL state", () => {
  it("writes only what differs from the defaults", () => {
    expect(todoStateToSearchParams(defaultTodoState()).toString()).toBe("");
  });
  it("round-trips", () => {
    const st = {
      scope: "assigned_by_me" as const, status: "all", filters: { q: "x", priority: "Urgent", due: "overdue" as const, user: "bia@x.com" },
      orderBy: "priority desc", page: 3, pageSize: 50, view: "kanban" as const,
    };
    expect(todoStateFromSearchParams(todoStateToSearchParams(st))).toEqual(st);
  });
  it("falls back to the defaults on unknown values", () => {
    const st = todoStateFromSearchParams(new URLSearchParams("scope=x&status=Nope&priority=Huge&due=later&view=gantt&page=-2&page_size=7"));
    expect(st).toEqual(defaultTodoState());
  });
});

describe("withTodoStatus", () => {
  it("changes only the named row", () => {
    const rows = [{ id: "a", status: "Open" }, { id: "b", status: "Open" }] as ToDoDoc[];
    expect(withTodoStatus(rows, "b", "Closed").map((r) => r.status)).toEqual(["Open", "Closed"]);
    expect(rows[1].status).toBe("Open");
  });
});

describe("countTodoFilters", () => {
  it("counts the filters that are set", () => {
    expect(countTodoFilters(emptyTodoFilters())).toBe(0);
    expect(countTodoFilters({ q: "  ", priority: "High", due: "today", user: "" })).toBe(2);
  });
});
