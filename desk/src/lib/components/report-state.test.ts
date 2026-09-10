import { describe, expect, it } from "vitest";
import type { Field } from "$lib/meta";
import { reportFiltersFromSearchParams, reportFiltersToSearchParams } from "./report-state";

const fields: Field[] = [
  { fieldname: "ate", fieldtype: "Date" },
  { fieldname: "imovel", fieldtype: "Link" },
  { fieldname: "somente_com_movimento", fieldtype: "Check" },
  { fieldname: "limite", fieldtype: "Currency" },
];

describe("report URL filters", () => {
  it("restaura somente filtros declarados e preserva seus tipos", () => {
    expect(reportFiltersFromSearchParams(
      new URLSearchParams("ate=2026-09-10&imovel=IMV-001&somente_com_movimento=false&limite=25.5&estranho=ignorar"),
      fields,
      { ate: "2026-09-30" },
    )).toEqual({ ate: "2026-09-10", imovel: "IMV-001", somente_com_movimento: false, limite: 25.5 });
  });

  it("mantém um filtro padrão vazio quando ele é removido em um link compartilhado", () => {
    expect(reportFiltersFromSearchParams(new URLSearchParams("ate="), fields, { ate: "2026-09-30" })).toEqual({ ate: "" });
  });

  it("serializa filtros ativos, inclusive falso e vazio", () => {
    expect(reportFiltersToSearchParams({ ate: "", imovel: "IMV-001", somente_com_movimento: false, limite: 25.5 }, fields).toString())
      .toBe("ate=&imovel=IMV-001&somente_com_movimento=false&limite=25.5");
  });
});
