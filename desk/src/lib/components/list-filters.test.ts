import { describe, expect, it } from "vitest";
import { buildListFilters } from "./list-filters";

describe("buildListFilters", () => {
  it("compares plain values by equality and skips empty ones", () => {
    expect(buildListFilters({ status: "Open", owner: "", priority: null })).toEqual([["status", "=", "Open"]]);
  });

  it("applies an extra option's filters in place of the equality", () => {
    const options = { status: [{ value: "late", label: "Late", filters: [["status", "=", "Open"], ["overdue", ">=", 1]] }] };
    expect(buildListFilters({ status: "late", owner: "a" }, options)).toEqual([
      ["status", "=", "Open"], ["overdue", ">=", 1], ["owner", "=", "a"],
    ]);
  });

  it("keeps the equality for a value that is not an extra option", () => {
    const options = { status: [{ value: "late", label: "Late", filters: [["overdue", ">=", 1]] }] };
    expect(buildListFilters({ status: "Open" }, options)).toEqual([["status", "=", "Open"]]);
  });
});
