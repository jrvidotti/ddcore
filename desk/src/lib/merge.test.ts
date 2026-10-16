import { describe, expect, it, vi } from "vitest";
import { canMerge, mergePreviewHtml } from "./merge";

vi.mock("./boot.svelte", () => ({ __: (s: string) => s, boot: {} }));

const __ = (s: string, a: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(a[+i]));

describe("canMerge", () => {
  const ok = { isNew: false, allowRename: true, perm: { write: true, delete: true }, docstatus: 0 };
  it("needs allowRename, write and delete on a saved, unsubmitted document", () => {
    expect(canMerge(ok)).toBe(true);
    expect(canMerge({ ...ok, docstatus: 2 })).toBe(true);
    expect(canMerge({ ...ok, isNew: true })).toBe(false);
    expect(canMerge({ ...ok, allowRename: false })).toBe(false);
    expect(canMerge({ ...ok, perm: { write: true } })).toBe(false);
    expect(canMerge({ ...ok, perm: { delete: true } })).toBe(false);
    expect(canMerge({ ...ok, docstatus: 1 })).toBe(false);
  });
});

describe("mergePreviewHtml", () => {
  it("lists what moves and escapes the ids", () => {
    const html = mergePreviewHtml({
      from: "<A>", into: "B",
      links: [{ doctype: "Employee", label: "Employee", fieldname: "unit", fieldLabel: "Unit", count: 23 }],
      records: [{ doctype: "Comment", label: "Comment", count: 2 }],
      blockers: [],
    }, __);
    expect(html).toContain("Employee");
    expect(html).toContain("23");
    expect(html).toContain("Comment");
    expect(html).toContain("&lt;A&gt;");
    expect(html).not.toContain("<A>");
  });
  it("shows the blockers and says when nothing links", () => {
    const html = mergePreviewHtml({ from: "A", into: "B", links: [], records: [], blockers: ["Store Unit must be unique"] }, __);
    expect(html).toContain("Store Unit must be unique");
    expect(html).toContain("Nothing links to");
  });
});
