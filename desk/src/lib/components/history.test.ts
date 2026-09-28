import { describe, expect, it } from "vitest";
import { diffTable, formatDiffValue, formatMultiSelect, parseVersion, signatureDiff } from "./history";

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

describe("Signature in history", () => {
  const a = "sha256:0123456789ab", b = "sha256:ba9876543210";
  const sig = { fieldname: "signed", label: "Signed by", fieldtype: "Signature" } as any;

  it("says what happened, never showing the marker", () => {
    expect(signatureDiff(null, a)).toEqual(["—", "Signed"]);
    expect(signatureDiff(a, null)).toEqual(["Signed", "Removed"]);
    expect(signatureDiff(a, b)).toEqual(["Signed", "Changed"]);
    expect(formatDiffValue(a, sig).formatted).toBe("Signed");
    expect(formatDiffValue(null, sig).formatted).toBe("—");
  });

  it("reads a Version's markers on the document and in child rows", () => {
    const frm: any = {
      field: (f: string) => (f === "signed" ? sig : f === "stops" ? { fieldname: "stops", fieldtype: "Table", options: "Stop", label: "Stops" } : undefined),
      meta: { children: { Stop: { name: "Stop", fields: [sig] } } },
    };
    const v = parseVersion({ id: "V-1", data: { changed: { signed: [a, b], stops: [[{ id: "r1", signed: null }], [{ id: "r1", signed: a }]] } } }, frm);
    const doc = v.changes.find((c) => c.field === "signed")!;
    expect([doc.formattedOld, doc.formattedNew]).toEqual(["Signed", "Changed"]);
    const row = v.changes.find((c) => c.field === "stops")!.tableDiff!.modified[0].changes[0];
    expect([row.formattedFrom, row.formattedTo]).toEqual(["—", "Signed"]);
    expect(JSON.stringify(v.changes.map((c) => [c.formattedOld, c.formattedNew]))).not.toContain("sha256");
  });
});
