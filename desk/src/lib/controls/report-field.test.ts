import { describe, expect, it } from "vitest";
import { reportFiltersFor } from "./report-field";

describe("reportFiltersFor", () => {
  it("takes each report filter from the document field it names", () => {
    expect(reportFiltersFor({ reportFilters: { course: "id", unit: "unit" } }, { id: "C-1", unit: "North" })).toEqual({ course: "C-1", unit: "North" });
  });
  it("sends null for an empty field and nothing without reportFilters", () => {
    expect(reportFiltersFor({ reportFilters: { unit: "unit" } }, { id: "C-1" })).toEqual({ unit: null });
    expect(reportFiltersFor({}, { id: "C-1" })).toEqual({});
  });
});
