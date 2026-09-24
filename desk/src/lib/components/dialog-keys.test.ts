import { describe, expect, it } from "vitest";
import { dialogKeyAction } from "./dialog-actions";

describe("dialog keys", () => {
  it("Escape cancels the dialog", () => {
    expect(dialogKeyAction({ key: "Escape", targetTag: "INPUT" })).toBe("cancel");
  });

  it("Enter in an input or on the dialog itself runs the primary action", () => {
    expect(dialogKeyAction({ key: "Enter", targetTag: "INPUT" })).toBe("primary");
    expect(dialogKeyAction({ key: "Enter", targetTag: "DIV" })).toBe("primary");
  });

  it("Enter keeps its meaning in a textarea or rich text, unless Ctrl/Cmd is held", () => {
    expect(dialogKeyAction({ key: "Enter", targetTag: "TEXTAREA" })).toBeNull();
    expect(dialogKeyAction({ key: "Enter", targetTag: "DIV", targetEditable: true })).toBeNull();
    expect(dialogKeyAction({ key: "Enter", targetTag: "TEXTAREA", metaKey: true })).toBe("primary");
    expect(dialogKeyAction({ key: "Enter", targetTag: "DIV", targetEditable: true, ctrlKey: true })).toBe("primary");
  });

  it("Enter on a focused button or link presses that, not the primary action", () => {
    expect(dialogKeyAction({ key: "Enter", targetTag: "BUTTON" })).toBeNull();
    expect(dialogKeyAction({ key: "Enter", targetTag: "A" })).toBeNull();
  });

  it("a key a control already handled, or an IME composition, is left alone", () => {
    expect(dialogKeyAction({ key: "Enter", targetTag: "INPUT", defaultPrevented: true })).toBeNull();
    expect(dialogKeyAction({ key: "Escape", targetTag: "INPUT", defaultPrevented: true })).toBeNull();
    expect(dialogKeyAction({ key: "Enter", targetTag: "INPUT", isComposing: true })).toBeNull();
  });

  it("a held-down, shifted or unrelated key does nothing", () => {
    expect(dialogKeyAction({ key: "Enter", targetTag: "INPUT", repeat: true })).toBeNull();
    expect(dialogKeyAction({ key: "Enter", targetTag: "INPUT", shiftKey: true })).toBeNull();
    expect(dialogKeyAction({ key: "a", targetTag: "INPUT" })).toBeNull();
  });

  it("Delete runs the danger action, except where text is typed", () => {
    expect(dialogKeyAction({ key: "Delete", targetTag: "DIV" })).toBe("danger");
    expect(dialogKeyAction({ key: "Delete", targetTag: "BUTTON" })).toBe("danger");
    expect(dialogKeyAction({ key: "Delete", targetTag: "INPUT" })).toBeNull();
    expect(dialogKeyAction({ key: "Delete", targetTag: "TEXTAREA" })).toBeNull();
    expect(dialogKeyAction({ key: "Delete", targetTag: "DIV", targetEditable: true })).toBeNull();
    expect(dialogKeyAction({ key: "Delete", targetTag: "DIV", repeat: true })).toBeNull();
  });
});
