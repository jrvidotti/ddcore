import { describe, expect, it } from "vitest";
import { exportBaseName, exportCell, exportTable, nextSort, sortRows } from "./grid-rows";

const cols: any[] = [
  { fieldname: "name", fieldtype: "Data", label: "Name" },
  { fieldname: "qty", fieldtype: "Int", label: "Qty" },
  { fieldname: "who", fieldtype: "Link", options: "Employee", label: "Who" },
  { fieldname: "status", fieldtype: "Select", options: ["Open", "Closed"], optionLabels: ["Aberto", "Fechado"], label: "Status" },
  { fieldname: "done", fieldtype: "Check", label: "Done" },
  { fieldname: "on", fieldtype: "Date", label: "On" },
];
const titles: Record<string, string> = { "E-1": "Zoe", "E-2": "Ana" };
const titleOf = (_: string, id: string) => titles[id];

describe("sortRows", () => {
  const rows = [
    { idx: 1, name: "b10", qty: 10, who: "E-1", status: "Open", on: "2026-03-01" },
    { idx: 2, name: "b9", qty: 9, who: "E-2", status: "Closed", on: "2025-12-31" },
    { idx: 3, name: "", qty: null, who: null, status: null, on: null },
    { idx: 4, name: "A", qty: 9, who: "E-2", status: "Open", on: "2026-01-15" },
  ];
  const ids = (rs: any[]) => rs.map((r) => r.idx);

  it("keeps the array order without a sort, as a new array of the same rows", () => {
    const out = sortRows(rows, null, cols);
    expect(out).not.toBe(rows);
    expect(out[0]).toBe(rows[0]);
    expect(ids(out)).toEqual([1, 2, 3, 4]);
  });
  it("sorts numbers numerically and is stable on ties, empty last both ways", () => {
    expect(ids(sortRows(rows, { field: "qty", order: "asc" }, cols))).toEqual([2, 4, 1, 3]);
    expect(ids(sortRows(rows, { field: "qty", order: "desc" }, cols))).toEqual([1, 2, 4, 3]);
  });
  it("sorts text naturally and case-insensitively", () => {
    expect(ids(sortRows(rows, { field: "name", order: "asc" }, cols))).toEqual([4, 2, 1, 3]);
  });
  it("sorts a Link by its title and a Select by its label", () => {
    expect(ids(sortRows(rows, { field: "who", order: "asc" }, cols, titleOf))).toEqual([2, 4, 1, 3]);
    expect(ids(sortRows(rows, { field: "status", order: "asc" }, cols))).toEqual([1, 4, 2, 3]);
  });
  it("sorts dates by their ISO text and idx as a number", () => {
    expect(ids(sortRows(rows, { field: "on", order: "asc" }, cols))).toEqual([2, 4, 1, 3]);
    expect(ids(sortRows(rows, { field: "idx", order: "desc" }, cols))).toEqual([4, 3, 2, 1]);
  });
});

describe("nextSort", () => {
  it("cycles ascending, descending, then back to the default", () => {
    expect(nextSort(null, "qty")).toEqual({ field: "qty", order: "asc" });
    expect(nextSort({ field: "qty", order: "asc" }, "qty")).toEqual({ field: "qty", order: "desc" });
    expect(nextSort({ field: "qty", order: "desc" }, "qty")).toBeNull();
    expect(nextSort({ field: "qty", order: "desc" }, "name")).toEqual({ field: "name", order: "asc" });
  });
  it("from the default sort, clicking its column descends, then comes back", () => {
    const base = { field: "name", order: "asc" as const };
    expect(nextSort(base, "name", base)).toEqual({ field: "name", order: "desc" });
    expect(nextSort({ field: "name", order: "desc" }, "name", base)).toBeNull();
  });
  it("never gets stuck when the default is descending", () => {
    const base = { field: "name", order: "desc" as const };
    expect(nextSort(base, "name", base)).toEqual({ field: "name", order: "asc" });
  });
});

describe("export", () => {
  it("exports titles, labels, numbers and booleans", () => {
    const row = { name: "x", qty: "3", who: "E-1", status: "Closed", done: 1, on: "2026-01-02" };
    expect(cols.map((c) => exportCell(row, c, titleOf))).toEqual(["x", 3, "Zoe", "Fechado", true, "2026-01-02"]);
    expect(exportCell({ who: "E-9" }, cols[2], titleOf)).toBe("E-9");
  });
  it("builds a header from the labels", () => {
    const t = exportTable([{ name: "a", qty: 1 }], cols.slice(0, 2));
    expect(t).toEqual({ header: ["Name", "Qty"], cells: [["a", 1]] });
  });
  it("makes a safe file name", () => {
    expect(exportBaseName("Course", "C/01: x", "students")).toBe("Course-C_01_ x-students");
    expect(exportBaseName(undefined, "")).toBe("export");
  });
});
