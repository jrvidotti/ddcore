import { describe, expect, it, vi } from "vitest";
import { actionRows, visibleActions } from "./list-actions";

const rows = [
  { id: "a", status: "Draft" },
  { id: "b", status: "Submitted" },
  { id: "c", status: "Draft" },
];

describe("actionRows", () => {
  it("returns the selected rows in list order when there is no condition", () => {
    expect(actionRows({ label: "Go", onClick() {} }, rows, new Set(["c", "a"])).map((r) => r.id)).toEqual(["a", "c"]);
  });

  it("keeps only the selected rows that pass the condition", () => {
    const action = { label: "Go", condition: (r: any) => r.status !== "Submitted", onClick() {} };
    expect(actionRows(action, rows, new Set(["a", "b"])).map((r) => r.id)).toEqual(["a"]);
  });

  it("counts a condition that throws as false", () => {
    const err = vi.spyOn(console, "error").mockImplementation(() => {});
    const action = { label: "Go", condition: (r: any) => { if (r.id === "a") throw new Error("boom"); return true; }, onClick() {} };
    expect(actionRows(action, rows, new Set(["a", "c"])).map((r) => r.id)).toEqual(["c"]);
    expect(err).toHaveBeenCalled();
    err.mockRestore();
  });
});

describe("visibleActions", () => {
  it("hides an action no selected row passes", () => {
    const submit = { label: "Submit", condition: (r: any) => r.status === "Draft", onClick() {} };
    const all = { label: "All", onClick() {} };
    const visible = visibleActions([submit, all], rows, new Set(["b"]));
    expect(visible.map((v) => v.action.label)).toEqual(["All"]);
    expect(visible[0].rows.map((r) => r.id)).toEqual(["b"]);
  });

  it("shows nothing without a selection", () => {
    expect(visibleActions([{ label: "All", onClick() {} }], rows, new Set())).toEqual([]);
  });
});
