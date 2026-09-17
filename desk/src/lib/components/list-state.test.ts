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
    expect(resolveAllowedViews({ views: ["calendar", "list"] })).toEqual(["calendar", "list"]);
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
