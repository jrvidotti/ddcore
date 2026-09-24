import { describe, expect, it } from "vitest";
import { diffTable, formatMultiSelect } from "./history";

const childMeta = {
  name: "Item",
  app: "x",
  label: "Item",
  fields: [{ fieldname: "qty", label: "Qty", fieldtype: "Int" }],
} as any;

describe("diffTable row identity", () => {
  it("pairs rows by id", () => {
    const d = diffTable(
      [{ id: "r1", idx: 1, qty: 1 }, { id: "r2", idx: 2, qty: 5 }],
      [{ id: "r2", idx: 1, qty: 6 }],
      childMeta,
    );
    expect(d.removed.map((r) => r.rowName)).toEqual(["r1"]);
    expect(d.added).toEqual([]);
    expect(d.modified.map((m) => m.rowName)).toEqual(["r2"]);
  });

  // A Version written before 0.17 kept its child rows keyed by `name`.
  it("pairs the rows of a Version written before 0.17 by their name", () => {
    const d = diffTable(
      [{ name: "r1", idx: 1, qty: 1 }, { name: "r2", idx: 2, qty: 5 }],
      [{ name: "r2", idx: 1, qty: 6 }],
      childMeta,
    );
    expect(d.removed.map((r) => r.rowName)).toEqual(["r1"]);
    expect(d.added).toEqual([]);
    expect(d.modified.map((m) => m.rowName)).toEqual(["r2"]);
    expect(d.modified[0].changes.map((c) => c.field)).toEqual(["qty"]);
  });
});

describe("Table MultiSelect in history", () => {
  const tagMeta = { name: "Note Tag", app: "x", label: "Note Tag", fields: [{ fieldname: "tag", label: "Tag", fieldtype: "Link", options: "Tag" }] } as any;

  it("shows the values, not a table of rows", () => {
    expect(formatMultiSelect([{ id: "a", tag: "red" }, { id: "b", tag: "blue" }], tagMeta)).toBe("red, blue");
    expect(formatMultiSelect([], tagMeta)).toBe("—");
  });
});
