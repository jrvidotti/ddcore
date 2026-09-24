import { describe, expect, it } from "vitest";
import { multiSelectLinkField, multiSelectValues, withValue, withoutValue } from "./multiselect-state";

const child: any = { name: "Note Tag", fields: [{ fieldname: "tag", fieldtype: "Link", options: "Tag" }, { fieldname: "note", fieldtype: "Data" }] };

describe("multiselect-state", () => {
  it("finds the one Link field, and nothing when there are two", () => {
    expect(multiSelectLinkField(child)?.fieldname).toBe("tag");
    expect(multiSelectLinkField({ ...child, fields: [...child.fields, { fieldname: "b", fieldtype: "Link" }] })).toBeNull();
    expect(multiSelectLinkField(undefined)).toBeNull();
  });

  it("reads values from rows and from plain ids", () => {
    expect(multiSelectValues([{ tag: "red" }, { tag: "" }, "blue"], "tag")).toEqual(["red", "blue"]);
    expect(multiSelectValues(null, "tag")).toEqual([]);
  });

  it("adds a value once, and keeps the rows that stay", () => {
    const rows = [{ id: "r1", tag: "red", idx: 1 }];
    const added = withValue(rows, "tag", "blue", "Note Tag");
    expect(added).toEqual([{ id: "r1", tag: "red", idx: 1 }, { doctype: "Note Tag", tag: "blue", __islocal: true, idx: 2 }]);
    expect(withValue(added, "tag", "red", "Note Tag")).toBe(added);
    expect(withValue(added, "tag", "", "Note Tag")).toBe(added);
  });

  it("removes a value and renumbers", () => {
    const rows = [{ id: "r1", tag: "red", idx: 1 }, { id: "r2", tag: "blue", idx: 2 }];
    expect(withoutValue(rows, "tag", "red")).toEqual([{ id: "r2", tag: "blue", idx: 1 }]);
  });
});
