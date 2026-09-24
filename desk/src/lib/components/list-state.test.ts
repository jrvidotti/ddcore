import { describe, expect, it } from "vitest";
import type { Field } from "$lib/meta";
import { clearListFilters, listStateFromSearchParams, listStateToSearchParams, resolveActiveView, resolveAllowedViews } from "./list-state";

const fields: Field[] = [
  { fieldname: "status", fieldtype: "Select" },
  { fieldname: "categoria", fieldtype: "Link" },
  { fieldname: "ativo", fieldtype: "Check" },
  { fieldname: "valor", fieldtype: "Currency" },
];

describe("list URL state", () => {
  it("restores typed filters and pagination from a shared link", () => {
    const state = listStateFromSearchParams(
      new URLSearchParams("status=Atrasado&categoria=CAT-1&ativo=false&valor=25.5&q=aluguel&docstatus=1&order_by=modified+desc&page=3&page_size=50"),
      fields,
      { filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20 },
    );

    expect(state).toEqual({
      filters: { status: "Atrasado", categoria: "CAT-1", ativo: false, valor: 25.5 },
      search: "aluguel",
      docstatusFilter: "1",
      orderBy: "modified desc",
      page: 3,
      pageSize: 50,
    });
  });

  it("restores a view from a shared link", () => {
    const state = listStateFromSearchParams(
      new URLSearchParams("view=calendar"),
      fields,
      { filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20 },
    );

    expect(state.view).toBe("calendar");
  });

  it("keeps the calendar's month, and only on the calendar", () => {
    const defaults = { filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20 };
    expect(listStateFromSearchParams(new URLSearchParams("view=calendar&month=2026-03"), fields, defaults).month).toBe("2026-03");
    expect(listStateFromSearchParams(new URLSearchParams("view=calendar&month=2026-13"), fields, defaults).month).toBeUndefined();
    expect(listStateToSearchParams({ ...defaults, view: "calendar", month: "2026-03" }, fields).toString()).toBe("page_size=20&view=calendar&month=2026-03");
    expect(listStateToSearchParams({ ...defaults, view: "list", month: "2026-03" }, fields).toString()).toBe("page_size=20");
  });

  it("serializes only list state into stable parameters", () => {
    const params = listStateToSearchParams({
      filters: { status: "Atrasado", categoria: "CAT-1", ativo: false, valor: 25.5 },
      search: "aluguel",
      docstatusFilter: "1",
      orderBy: "modified desc",
      page: 3,
      pageSize: 50,
    }, fields);

    expect(params.toString()).toBe("status=Atrasado&categoria=CAT-1&ativo=false&valor=25.5&q=aluguel&docstatus=1&order_by=modified+desc&page=3&page_size=50");
  });

  it("serializes non-list views but omits the default list view", () => {
    const calendar = listStateToSearchParams({
      filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20, view: "calendar",
    }, fields);
    const list = listStateToSearchParams({
      filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20, view: "list",
    }, fields);

    expect(calendar.toString()).toBe("page_size=20&view=calendar");
    expect(list.toString()).toBe("page_size=20");
  });

  it("includes default size so a link does not inherit another list configuration", () => {
    const params = listStateToSearchParams({
      filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20,
    }, fields);

    expect(params.toString()).toBe("page_size=20");
  });

  it("ignores invalid pagination and parameters that are not DocType filters", () => {
    const state = listStateFromSearchParams(
      new URLSearchParams("estranho=valor&page=0&page_size=999"),
      fields,
      { filters: { status: "Pendente" }, search: "", docstatusFilter: "", orderBy: "", page: 2, pageSize: 20 },
    );

    expect(state).toEqual({ filters: { status: "Pendente" }, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20 });
  });

  it("clears filters and search without altering ordering or page size", () => {
    expect(clearListFilters({
      filters: { status: "Atrasado", categoria: "CAT-1" }, search: "aluguel", docstatusFilter: "2", orderBy: "modified desc", page: 4, pageSize: 100,
    })).toEqual({ filters: {}, search: "", docstatusFilter: "", orderBy: "modified desc", page: 1, pageSize: 100 });
  });
});

describe("view resolution", () => {
  it("defaults allowed views to list and cards when no calendar configured", () => {
    expect(resolveAllowedViews({})).toEqual(["list", "cards"]);
  });

  it("includes calendar in allowed views when calendar field configured", () => {
    expect(resolveAllowedViews({ calendar: { field: "due_date" } })).toEqual(["list", "calendar", "cards"]);
  });

  it("adds kanban and gantt only when fully configured", () => {
    expect(resolveAllowedViews({ calendar: { field: "due_date" }, kanban: { field: "status" }, gantt: { startField: "start_date", endField: "due_date" } }))
      .toEqual(["list", "calendar", "kanban", "gantt", "cards"]);
    expect(resolveAllowedViews({ gantt: { startField: "start_date" } })).toEqual(["list", "cards"]);
  });

  it("respects explicit views array from settings", () => {
    expect(resolveAllowedViews({ views: ["calendar", "list"], calendar: { field: "due_date" } })).toEqual(["calendar", "list"]);
  });

  it("drops an explicit calendar view without a configured field", () => {
    expect(resolveAllowedViews({ views: ["list", "calendar"] })).toEqual(["list"]);
  });

  it("drops an explicit gantt view configured with only a start field", () => {
    expect(resolveAllowedViews({ views: ["list", "gantt"], gantt: { startField: "start_date" } })).toEqual(["list"]);
  });

  it("keeps an explicit kanban view when its field is configured, preserving the given order", () => {
    expect(resolveAllowedViews({ views: ["kanban", "list"], kanban: { field: "status" } })).toEqual(["kanban", "list"]);
  });

  it("keeps explicit list and cards views without any other settings", () => {
    expect(resolveAllowedViews({ views: ["cards", "list"] })).toEqual(["cards", "list"]);
  });

  it("resolves active view giving highest priority to URL param", () => {
    expect(resolveActiveView(["list", "cards", "calendar"], "calendar", "cards", false)).toBe("calendar");
  });

  it("resolves active view falling back to stored view if URL param is absent or invalid", () => {
    expect(resolveActiveView(["list", "cards"], "unknown", "cards", false)).toBe("cards");
    expect(resolveActiveView(["list", "cards"], null, "cards", false)).toBe("cards");
  });

  it("defaults to cards on mobile (< 768px) when no URL or stored view exists", () => {
    expect(resolveActiveView(["list", "cards"], null, null, true)).toBe("cards");
  });

  it("defaults to primary view (first item) on desktop when no URL or stored view exists", () => {
    expect(resolveActiveView(["calendar", "list", "cards"], null, null, false)).toBe("calendar");
    expect(resolveActiveView(["list", "cards"], null, null, false)).toBe("list");
  });
});

describe("tree view", () => {
  it("is offered only to a tree DocType, and first", () => {
    expect(resolveAllowedViews({}, true)).toEqual(["tree", "list", "cards"]);
    expect(resolveAllowedViews({}, false)).toEqual(["list", "cards"]);
  });

  it("is dropped from an explicit list when the DocType is not a tree", () => {
    expect(resolveAllowedViews({ views: ["tree", "list"] }, false)).toEqual(["list"]);
    expect(resolveAllowedViews({ views: ["list", "tree"] }, true)).toEqual(["list", "tree"]);
  });

  it("is what a phone opens, ahead of cards", () => {
    expect(resolveActiveView(["tree", "list", "cards"], null, null, true)).toBe("tree");
    expect(resolveActiveView(["list", "cards"], null, null, true)).toBe("cards");
  });

  it("still yields to an explicit choice in the URL", () => {
    expect(resolveActiveView(["tree", "list", "cards"], "list", null, true)).toBe("list");
  });
});
