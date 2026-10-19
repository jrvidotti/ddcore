import { describe, it, expect } from "vitest";
import { colorStyle, docStatus, docstatusName, statusColor } from "./format";

// What this replaced was a regex over Portuguese word stems: "concluído" was
// green and "completed" was grey. Colour must never be decided by the text a
// human happens to be reading, so these tests check the rules and check that
// no language is anywhere in them.

describe("statusColor", () => {
  it("takes the field's optionColors first, keyed by the canonical value", () => {
    const field = { optionColors: { Open: "purple", Completed: "gray" } };
    expect(statusColor("Open", field)).toBe("purple");
    expect(statusColor("Completed", field)).toBe("gray");
  });

  it("falls back to the framework's canonical statuses", () => {
    expect(statusColor("Draft")).toBe("orange");
    expect(statusColor("Open")).toBe("blue");
    expect(statusColor("In progress")).toBe("blue");
    expect(statusColor("Completed")).toBe("green");
    expect(statusColor("Paid")).toBe("green");
    expect(statusColor("Overdue")).toBe("red");
    expect(statusColor("Cancelled")).toBe("red");
  });

  it("matches the canonical list case-insensitively", () => {
    expect(statusColor("draft")).toBe("orange");
    expect(statusColor("COMPLETED")).toBe("green");
  });

  it("hashes an unknown value into a stable colour", () => {
    const a = statusColor("Awaiting shipment");
    expect(a).toBe(statusColor("Awaiting shipment"));
    expect(["blue", "green", "orange", "red", "purple", "gray"]).toContain(a);
    // two different values are allowed to differ; neither may be undefined
    expect(statusColor("Somewhere else")).toBeTruthy();
  });

  it("is grey for nothing at all", () => {
    expect(statusColor("")).toBe("gray");
    expect(statusColor(undefined as any)).toBe("gray");
  });

  it("no longer reads Portuguese word stems", () => {
    // "concluído" used to be green through a regex; it is now just an unknown
    // value, and an app that wants it coloured declares optionColors.
    expect(statusColor("Concluído")).not.toBe(statusColor("Completed"));
  });
});

describe("colorStyle", () => {
  const color = { fieldtype: "Color" };

  it("paints with a Color field's own value", () => {
    const style = colorStyle("#f97316", color)!;
    expect(style).toContain("background: color-mix(in srgb, #f97316");
    expect(style).toContain("color: color-mix(in srgb, #f97316");
    // and the class underneath is the neutral one, never a hash of the hex
    expect(statusColor("#f97316", color)).toBe("gray");
  });

  it("leaves every other field to the palette", () => {
    expect(colorStyle("#f97316", { fieldtype: "Select" })).toBeUndefined();
    expect(colorStyle("Open")).toBeUndefined();
  });

  it("ignores what is not a colour", () => {
    expect(colorStyle("", color)).toBeUndefined();
    expect(colorStyle("orange", color)).toBeUndefined();
    expect(colorStyle("#f97316; position: fixed", color)).toBeUndefined();
  });
});

describe("docStatus", () => {
  const select = { fieldname: "status", fieldtype: "Select", options: ["Open", "Closed"], optionLabels: ["Aberto", "Fechado"] } as any;

  it("shows the status field's value and its label", () => {
    expect(docStatus({ status: "Closed" }, select, true)).toEqual({ value: "Closed", label: "Fechado" });
    expect(docStatus({ status: "Other" }, select, false)).toEqual({ value: "Other", label: "Other" });
  });

  it("falls back to the docstatus of a submittable DocType", () => {
    expect(docStatus({ docstatus: 1 }, select, true)).toEqual({ value: "Submitted", label: "Submitted" });
    expect(docStatus({ docstatus: 2 }, undefined, true).value).toBe("Cancelled");
    expect(docStatus({ docstatus: 0 }, undefined, true).value).toBe("Draft");
    expect(docStatus({ docstatus: 1 }, undefined, false)).toEqual({ value: "", label: "" });
  });

  it("calls an unsaved document New, whatever its DocType", () => {
    expect(docStatus({}, undefined, true, true).value).toBe("New");
    expect(docStatus({ status: "Open" }, select, true, true).value).toBe("Open");
  });
});

describe("docstatusName", () => {
  it("names 0, 1 and 2", () => {
    expect([0, 1, 2, "1", undefined].map(docstatusName)).toEqual(["Draft", "Submitted", "Cancelled", "Submitted", "Draft"]);
  });
});
