import assert from "node:assert/strict";
import { describe, expect, it, test } from "vitest";
import {
  commitFocusedEdit,
  getModifierKey,
  getModifierName,
  getShortcutsList,
  isEditableElement,
  isMac,
  shouldSave,
  shouldToggleShortcuts,
} from "./shortcuts";

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

describe("shouldSave", () => {
  it("accepts Ctrl+S and Cmd+S", () => {
    expect(shouldSave({ key: "s", ctrlKey: true })).toBe(true);
    expect(shouldSave({ key: "s", metaKey: true })).toBe(true);
  });

  // Windows reports the letter in upper case while Caps Lock is on; missing it
  // lets the browser's own "Save page as" dialog through
  it("accepts the shortcut with Caps Lock on", () => {
    expect(shouldSave({ key: "S", ctrlKey: true })).toBe(true);
  });

  it("ignores a bare S and other modifier combinations", () => {
    expect(shouldSave({ key: "s" })).toBe(false);
    expect(shouldSave({ key: "S", ctrlKey: true, shiftKey: true })).toBe(false);
    expect(shouldSave({ key: "s", ctrlKey: true, altKey: true })).toBe(false);
  });
});

test("isMac accurately detects Apple platforms", () => {
  assert.equal(isMac("MacIntel"), true);
  assert.equal(isMac("Macintosh; Intel Mac OS X 10_15_7"), true);
  assert.equal(isMac("iPhone"), true);
  assert.equal(isMac("iPad"), true);
  assert.equal(isMac("Win32"), false);
  assert.equal(isMac("Windows NT 10.0; Win64; x64"), false);
  assert.equal(isMac("Linux x86_64"), false);
});

test("getModifierKey returns ⌘ for Mac and Ctrl for other platforms", () => {
  assert.equal(getModifierKey(true), "⌘");
  assert.equal(getModifierKey(false), "Ctrl");
});

test("getModifierName returns Command for Mac and Ctrl for other platforms", () => {
  assert.equal(getModifierName(true), "Command");
  assert.equal(getModifierName(false), "Ctrl");
});

test("isEditableElement identifies inputs, textareas, selects and contenteditable elements", () => {
  assert.equal(isEditableElement(el({ tagName: "INPUT" })), true);
  assert.equal(isEditableElement(el({ tagName: "input" })), true);
  assert.equal(isEditableElement(el({ tagName: "TEXTAREA" })), true);
  assert.equal(isEditableElement(el({ tagName: "SELECT" })), true);
  assert.equal(isEditableElement(el({ tagName: "DIV", isContentEditable: true })), true);

  assert.equal(isEditableElement(el({ tagName: "BUTTON" })), false);
  assert.equal(isEditableElement(el({ tagName: "DIV" })), false);
  assert.equal(isEditableElement(el({ tagName: "BODY" })), false);
  assert.equal(isEditableElement(null), false);
  assert.equal(isEditableElement(undefined as unknown as EventTarget), false);
});

test("shouldToggleShortcuts triggers on '?' only when outside editable fields", () => {
  assert.equal(shouldToggleShortcuts({ key: "?", target: el({ tagName: "BUTTON" }) }), true);
  assert.equal(shouldToggleShortcuts({ key: "?", target: el({ tagName: "BODY" }) }), true);
  assert.equal(shouldToggleShortcuts({ key: "?", target: null }), true);

  // When focused on an input or textarea, '?' should not open the modal
  assert.equal(shouldToggleShortcuts({ key: "?", target: el({ tagName: "INPUT" }) }), false);
  assert.equal(shouldToggleShortcuts({ key: "?", target: el({ tagName: "TEXTAREA" }) }), false);
});

test("shouldToggleShortcuts triggers on Cmd+/ or Ctrl+/ even inside inputs", () => {
  assert.equal(shouldToggleShortcuts({ key: "/", metaKey: true, target: el({ tagName: "INPUT" }) }), true);
  assert.equal(shouldToggleShortcuts({ key: "/", ctrlKey: true, target: el({ tagName: "TEXTAREA" }) }), true);
  assert.equal(shouldToggleShortcuts({ key: "/", metaKey: true, target: null }), true);

  // Regular '/' does not trigger
  assert.equal(shouldToggleShortcuts({ key: "/", target: el({ tagName: "BODY" }) }), false);
});

test("getShortcutsList returns categorized shortcuts with platform modifier", () => {
  const macList = getShortcutsList(true);
  const winList = getShortcutsList(false);

  assert.equal(macList.length >= 3, true);

  const macForm = macList.find((g) => g.category.includes("Form"));
  assert.ok(macForm);
  const macSave = macForm!.shortcuts.find((s) => s.description.toLowerCase().includes("save"));
  assert.ok(macSave);
  assert.deepEqual(macSave!.keys, ["⌘", "S"]);

  const winForm = winList.find((g) => g.category.includes("Form"));
  assert.ok(winForm);
  const winSave = winForm!.shortcuts.find((s) => s.description.toLowerCase().includes("save"));
  assert.ok(winSave);
  assert.deepEqual(winSave!.keys, ["Ctrl", "S"]);
});
