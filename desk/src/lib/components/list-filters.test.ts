import { describe, expect, it } from "vitest";
import { buildListFilters, formatRange, parseRange, rangeFilters, tableFilterLink, tableFilters } from "./list-filters";

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

describe("table filters", () => {
  const children = {
    "Has Role": { name: "Has Role", fields: [{ fieldname: "role", fieldtype: "Link", options: "Role", label: "Role", reqd: true }] },
    "Order Line": { name: "Order Line", fields: [{ fieldname: "item", fieldtype: "Link", options: "Item" }, { fieldname: "warehouse", fieldtype: "Link", options: "Warehouse" }] },
  } as any;
  const roles = { fieldname: "roles", fieldtype: "Table", options: "Has Role", label: "Roles" };

  it("filters a table by the one Link its rows hold", () => {
    expect(tableFilterLink(roles, children)).toEqual({ path: "Has Role.role", link: children["Has Role"].fields[0] });
    expect(tableFilters(roles, children, "System Manager")).toEqual([["Has Role.role", "=", "System Manager"]]);
    expect(tableFilters({ ...roles, fieldtype: "Table MultiSelect" }, children, "All")).toEqual([["Has Role.role", "=", "All"]]);
  });

  it("has nothing to filter by without exactly one Link, a value or a table", () => {
    expect(tableFilterLink({ fieldname: "lines", fieldtype: "Table", options: "Order Line" }, children)).toBeNull();
    expect(tableFilterLink({ fieldname: "lines", fieldtype: "Table", options: "Missing" }, children)).toBeNull();
    expect(tableFilterLink({ fieldname: "role", fieldtype: "Link", options: "Role" }, children)).toBeNull();
    expect(tableFilters(roles, children, "")).toEqual([]);
  });
});

describe("range filters", () => {
  const fields = [
    { fieldname: "available_on", fieldtype: "Date" },
    { fieldname: "event_at", fieldtype: "Datetime" },
  ] as any[];

  it("reads either side of from..to, and a bare day as that day alone", () => {
    expect(parseRange("2026-10-01..2026-10-31")).toEqual({ from: "2026-10-01", to: "2026-10-31" });
    expect(parseRange("2026-10-01..")).toEqual({ from: "2026-10-01", to: "" });
    expect(parseRange("..2026-10-31")).toEqual({ from: "", to: "2026-10-31" });
    expect(parseRange("2026-10-05")).toEqual({ from: "2026-10-05", to: "2026-10-05" });
    expect(parseRange("soon..2026-13")).toEqual({ from: "", to: "" });
    expect(parseRange(undefined)).toEqual({ from: "", to: "" });
  });

  it("writes back what it reads, and nothing for an empty range", () => {
    expect(formatRange("2026-10-01", "")).toBe("2026-10-01..");
    expect(formatRange(null, "2026-10-31")).toBe("..2026-10-31");
    expect(formatRange("", null)).toBe("");
  });

  it("bounds a Date by the days themselves, each side only when set", () => {
    expect(rangeFilters("available_on", fields, "2026-10-01..2026-10-31")).toEqual([
      ["available_on", ">=", "2026-10-01"], ["available_on", "<=", "2026-10-31"],
    ]);
    expect(rangeFilters("available_on", fields, "2026-10-01..")).toEqual([["available_on", ">=", "2026-10-01"]]);
    expect(rangeFilters("available_on", fields, "..2026-10-31")).toEqual([["available_on", "<=", "2026-10-31"]]);
    expect(rangeFilters("available_on", fields, "")).toEqual([]);
  });

  it("bounds a Datetime from the first day's first instant to the last day's last, in the site's zone", () => {
    expect(rangeFilters("event_at", fields, "2026-10-01..2026-10-31", "America/Sao_Paulo")).toEqual([
      ["event_at", ">=", "2026-10-01T03:00:00.000Z"], ["event_at", "<=", "2026-11-01T02:59:59.999999Z"],
    ]);
    expect(rangeFilters("event_at", fields, "..2026-10-31", "America/Sao_Paulo")).toEqual([
      ["event_at", "<=", "2026-11-01T02:59:59.999999Z"],
    ]);
  });
});
