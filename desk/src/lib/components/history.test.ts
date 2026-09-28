import { describe, expect, it } from "vitest";
import { diffTable, formatMultiSelect, parseVersion } from "./history";

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

describe("parseVersion", () => {
  it("marks the Version a delete wrote, whose data is the document rather than a diff", () => {
    const v = parseVersion({ id: "v1", owner: "Admin", creation: "2026-09-28 10:00:00", data: { deleted: { id: "m1", title: "m1" } } });
    expect(v.deleted).toBe(true);
    expect(v.changes).toEqual([]);
  });

  it("leaves an ordinary Version unmarked", () => {
    const v = parseVersion({ id: "v2", owner: "Admin", creation: "2026-09-28 10:00:00", data: JSON.stringify({ changed: { title: ["a", "b"] } }) });
    expect(v.deleted).toBe(false);
    expect(v.changes.map((c) => c.field)).toEqual(["title"]);
  });
});
