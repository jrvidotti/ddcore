import { describe, expect, it } from "vitest";
import { commitFocusedEdit } from "./shortcuts";

/** The focused element, as much of it as `commitFocusedEdit` looks at. */
const el = (fake: { tagName?: string; isContentEditable?: boolean; blur?: () => void }) =>
  fake as unknown as EventTarget;

describe("commitFocusedEdit", () => {
  it("flushes the field being typed, which only commits when focus leaves it", () => {
    // a control that commits on change/blur, like a textarea or a number input
    const doc: Record<string, any> = { notes: "old" };
    const typed = "new";

    expect(commitFocusedEdit(el({ tagName: "TEXTAREA", blur: () => (doc.notes = typed) }))).toBe(true);
    expect(doc.notes).toBe("new");
  });

  it("commits inputs, selects and contenteditable elements", () => {
    for (const field of [
      { tagName: "INPUT" },
      { tagName: "SELECT" },
      { tagName: "DIV", isContentEditable: true },
    ]) {
      let blurred = false;
      expect(commitFocusedEdit(el({ ...field, blur: () => (blurred = true) }))).toBe(true);
      expect(blurred).toBe(true);
    }
  });

  it("leaves anything that is not an editable field alone", () => {
    let blurred = false;
    const blur = () => (blurred = true);
    expect(commitFocusedEdit(el({ tagName: "BODY", blur }))).toBe(false);
    expect(commitFocusedEdit(el({ tagName: "BUTTON", blur }))).toBe(false);
    expect(commitFocusedEdit(null)).toBe(false);
    expect(blurred).toBe(false);
  });
});
