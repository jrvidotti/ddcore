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
  it("restaura filtros tipados e paginação de um link compartilhado", () => {
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

  it("serializa somente o estado da lista em parâmetros estáveis", () => {
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

  it("inclui o tamanho padrão para que um link não herde a configuração de outra lista", () => {
    const params = listStateToSearchParams({
      filters: {}, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20,
    }, fields);

    expect(params.toString()).toBe("page_size=20");
  });

  it("ignora paginação inválida e parâmetros que não são filtros do DocType", () => {
    const state = listStateFromSearchParams(
      new URLSearchParams("estranho=valor&page=0&page_size=999"),
      fields,
      { filters: { status: "Pendente" }, search: "", docstatusFilter: "", orderBy: "", page: 2, pageSize: 20 },
    );

    expect(state).toEqual({ filters: { status: "Pendente" }, search: "", docstatusFilter: "", orderBy: "", page: 1, pageSize: 20 });
  });

  it("limpa filtros e busca sem alterar ordenação ou tamanho da página", () => {
    expect(clearListFilters({
      filters: { status: "Atrasado", categoria: "CAT-1" }, search: "aluguel", docstatusFilter: "2", orderBy: "modified desc", page: 4, pageSize: 100,
    })).toEqual({ filters: {}, search: "", docstatusFilter: "", orderBy: "modified desc", page: 1, pageSize: 100 });
  });
});
