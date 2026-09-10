import { describe, it, expect } from "vitest";
import { statusColor } from "./format";

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
