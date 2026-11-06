import { describe, expect, it } from "vitest";
import type { Field } from "$lib/meta";
import { drillFilters, isDrill, reportFiltersFromSearchParams, reportFiltersToSearchParams } from "./report-state";

const fields: Field[] = [
  { fieldname: "ate", fieldtype: "Date" },
  { fieldname: "imovel", fieldtype: "Link" },
  { fieldname: "somente_com_movimento", fieldtype: "Check" },
  { fieldname: "limite", fieldtype: "Currency" },
];

describe("report URL filters", () => {
  it("restores only declared filters and preserves their types", () => {
    expect(reportFiltersFromSearchParams(
      new URLSearchParams("ate=2026-09-10&imovel=IMV-001&somente_com_movimento=false&limite=25.5&estranho=ignorar"),
      fields,
      { ate: "2026-09-30" },
    )).toEqual({ ate: "2026-09-10", imovel: "IMV-001", somente_com_movimento: false, limite: 25.5 });
  });

  it("keeps a default filter empty when removed in a shared link", () => {
    expect(reportFiltersFromSearchParams(new URLSearchParams("ate="), fields, { ate: "2026-09-30" })).toEqual({ ate: "" });
  });

  it("serializes active filters, including false and empty", () => {
    expect(reportFiltersToSearchParams({ ate: "", imovel: "IMV-001", somente_com_movimento: false, limite: 25.5 }, fields).toString())
      .toBe("ate=&imovel=IMV-001&somente_com_movimento=false&limite=25.5");
  });
});

describe("report drill-down", () => {
  const current = { ate: "2026-09-30", imovel: "IMV-001", limite: 10 };

  it("puts a drill's filters over the current ones", () => {
    expect(drillFilters(current, { imovel: "IMV-002", somente_com_movimento: true }, fields))
      .toEqual({ ate: "2026-09-30", imovel: "IMV-002", limite: 10, somente_com_movimento: true });
  });

  it("empties a filter the drill sets to null, and takes no filter the report lacks", () => {
    expect(drillFilters(current, { imovel: null, estranho: "x" }, fields)).toEqual({ ate: "2026-09-30", imovel: "", limite: 10 });
  });

  it("is no drill without one of the report's filters", () => {
    for (const d of [undefined, null, "imovel", ["imovel"], {}, { estranho: "x" }]) {
      expect(isDrill(d, fields)).toBe(false);
      expect(drillFilters(current, d, fields)).toBeNull();
    }
    expect(isDrill({ limite: 5 }, fields)).toBe(true);
  });
});
