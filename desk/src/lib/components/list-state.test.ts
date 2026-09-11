import { describe, expect, it } from "vitest";
import type { Field } from "$lib/meta";
import { clearListFilters, listStateFromSearchParams, listStateToSearchParams } from "./list-state";

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
