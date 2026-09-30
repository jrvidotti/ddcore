import { describe, expect, it } from "vitest";
import { exportBaseName, exportCell, exportTable, filterRows, filterTuples, nextSort, searchRows, sortRows } from "./grid-rows";

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
  it("says a signature is there instead of exporting its data URL", () => {
    const sig = { fieldname: "signed", fieldtype: "Signature", label: "Signed" } as any;
    expect(exportCell({ signed: "data:image/png;base64,AAAA" }, sig)).toBe("Signed");
    expect(exportCell({ signed: null }, sig)).toBe("");
  });
  it("exports and sorts a Geolocation as its summary", () => {
    const geo = { fieldname: "place", fieldtype: "Geolocation", label: "Place" } as any;
    const at = (lon: number, lat: number) => ({ type: "Point", coordinates: [lon, lat] });
    expect(exportCell({ place: at(-46.6333, -23.5505) }, geo)).toBe("-23.55050, -46.63330");
    expect(exportCell({ place: null }, geo)).toBe("");
    const rows = [{ place: at(0, 30) }, { place: null }, { place: at(0, 10) }];
    expect(sortRows(rows, { field: "place", order: "asc" }, [geo]).map((r) => r.place?.coordinates[1] ?? null)).toEqual([10, 30, null]);
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

describe("grid filters", () => {
  const cols: any[] = [
    { fieldname: "in_class", fieldtype: "Check" },
    { fieldname: "needs_course", fieldtype: "Check" },
    { fieldname: "grade", fieldtype: "Float" },
    { fieldname: "name", fieldtype: "Data" },
    { fieldname: "on", fieldtype: "Date" },
  ];
  const rows = [
    { idx: 1, name: "Ana Lima", in_class: 1, needs_course: false, grade: 9, on: "2026-01-10" },
    { idx: 2, name: "Bia", in_class: 0, needs_course: true, grade: 5, on: "2026-03-01" },
    { idx: 3, name: "Caio", in_class: true, needs_course: true, grade: null, on: null },
    { idx: 4, name: "Duda", needs_course: null, grade: 7, on: "2026-02-15" },
  ];
  const ids = (rs: any[]) => rs.map((r) => r.idx);

  it("reads tuples, [field, value] pairs and objects", () => {
    expect(filterTuples([["a", "=", 1], ["b", 2], ["c", ["x", "y"]]])).toEqual([["a", "=", 1], ["b", "=", 2], ["c", "in", ["x", "y"]]]);
    expect(filterTuples({ a: 1, b: ["x"] })).toEqual([["a", "=", 1], ["b", "in", ["x"]]]);
    expect(filterTuples(null)).toEqual([]);
  });
  it("compares a Check as true or false, whatever form the value takes", () => {
    expect(ids(filterRows(rows, [[["in_class", "=", 1]]], cols))).toEqual([1, 3]);
    expect(ids(filterRows(rows, [{ needs_course: 1 }], cols))).toEqual([2, 3]);
    expect(ids(filterRows(rows, [[["in_class", "=", 0]]], cols))).toEqual([2, 4]);
    expect(ids(filterRows(rows, [[["needs_course", "!=", true]]], cols))).toEqual([1, 4]);
  });
  it("combines active filters with AND and returns the rows untouched without one", () => {
    expect(ids(filterRows(rows, [[["in_class", "=", 1]], { needs_course: 1 }], cols))).toEqual([3]);
    expect(filterRows(rows, [], cols)).toBe(rows);
  });
  it("orders numbers and dates, leaving empty values out", () => {
    expect(ids(filterRows(rows, [[["grade", ">=", 7]]], cols))).toEqual([1, 4]);
    expect(ids(filterRows(rows, [[["grade", "<", "7"]]], cols))).toEqual([2]);
    expect(ids(filterRows(rows, [[["on", "between", ["2026-01-01", "2026-02-28"]]]], cols))).toEqual([1, 4]);
  });
  it("matches like, in, is set and not set", () => {
    expect(ids(filterRows(rows, [[["name", "like", "%li%"]]], cols))).toEqual([1]);
    expect(ids(filterRows(rows, [[["name", "not like", "b%"]]], cols))).toEqual([1, 3, 4]);
    expect(ids(filterRows(rows, [[["name", "in", "Bia, Duda"]]], cols))).toEqual([2, 4]);
    expect(ids(filterRows(rows, [[["grade", "not in", [5, 9]]]], cols))).toEqual([3, 4]);
    expect(ids(filterRows(rows, [[["on", "is", "set"]]], cols))).toEqual([1, 2, 4]);
    expect(ids(filterRows(rows, [[["grade", "not set", null]]], cols))).toEqual([3]);
  });
});

describe("grid search", () => {
  const rows = [
    { id: 1, name: "João Árvore", who: "E-1", status: "Open", done: 1, unit: "Matriz" },
    { id: 2, name: "Maria", who: "E-2", status: "Closed", done: 0, unit: "Filial São Paulo" },
    { id: 3, name: "Bia", who: null, status: "", done: 1, unit: "" },
  ];
  const ids = (rs: any[]) => rs.map((r) => r.id);
  const search = (q: string, fields = ["name", "who", "status", "done", "unit"]) => ids(searchRows(rows, q, fields, cols, titleOf));

  it("matches a part of the text, ignoring case and accents", () => {
    expect(search("joao")).toEqual([1]);
    expect(search("ARV")).toEqual([1]);
    expect(search("sao paulo")).toEqual([2]);
    expect(search("filial são")).toEqual([2]);
  });
  it("finds a Link by its id and its title, a Select by its value and its label", () => {
    expect(search("zoe")).toEqual([1]);
    expect(search("e-2")).toEqual([2]);
    expect(search("fechado")).toEqual([2]);
    expect(search("closed")).toEqual([2]);
  });
  it("needs every word, each in any of the columns, and only in the columns named", () => {
    expect(search("maria filial")).toEqual([2]);
    expect(search("maria matriz")).toEqual([]);
    expect(search("matriz", ["name"])).toEqual([]);
    // a Check has no text to find
    expect(search("1", ["done"])).toEqual([]);
  });
  it("keeps every row for a blank query or no columns", () => {
    expect(searchRows(rows, "  ", ["name"], cols)).toBe(rows);
    expect(searchRows(rows, "bia", [], cols)).toBe(rows);
  });
});
