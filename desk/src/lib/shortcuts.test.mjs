import assert from "node:assert/strict";
import test from "node:test";

import {
  isMac,
  getModifierKey,
  getModifierName,
  isEditableElement,
  shouldToggleShortcuts,
  getShortcutsList,
} from "./shortcuts.ts";

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
  assert.equal(isEditableElement({ tagName: "INPUT" }), true);
  assert.equal(isEditableElement({ tagName: "input" }), true);
  assert.equal(isEditableElement({ tagName: "TEXTAREA" }), true);
  assert.equal(isEditableElement({ tagName: "SELECT" }), true);
  assert.equal(isEditableElement({ tagName: "DIV", isContentEditable: true }), true);

  assert.equal(isEditableElement({ tagName: "BUTTON" }), false);
  assert.equal(isEditableElement({ tagName: "DIV" }), false);
  assert.equal(isEditableElement({ tagName: "BODY" }), false);
  assert.equal(isEditableElement(null), false);
  assert.equal(isEditableElement(undefined), false);
});

test("shouldToggleShortcuts triggers on '?' only when outside editable fields", () => {
  assert.equal(shouldToggleShortcuts({ key: "?", target: { tagName: "BUTTON" } }), true);
  assert.equal(shouldToggleShortcuts({ key: "?", target: { tagName: "BODY" } }), true);
  assert.equal(shouldToggleShortcuts({ key: "?", target: null }), true);

  // When focused on an input or textarea, '?' should not open the modal
  assert.equal(shouldToggleShortcuts({ key: "?", target: { tagName: "INPUT" } }), false);
  assert.equal(shouldToggleShortcuts({ key: "?", target: { tagName: "TEXTAREA" } }), false);
});

test("shouldToggleShortcuts triggers on Cmd+/ or Ctrl+/ even inside inputs", () => {
  assert.equal(shouldToggleShortcuts({ key: "/", metaKey: true, target: { tagName: "INPUT" } }), true);
  assert.equal(shouldToggleShortcuts({ key: "/", ctrlKey: true, target: { tagName: "TEXTAREA" } }), true);
  assert.equal(shouldToggleShortcuts({ key: "/", metaKey: true, target: null }), true);

  // Regular '/' does not trigger
  assert.equal(shouldToggleShortcuts({ key: "/", target: { tagName: "BODY" } }), false);
});

test("getShortcutsList returns categorized shortcuts with platform modifier", () => {
  const macList = getShortcutsList(true);
  const winList = getShortcutsList(false);

  assert.equal(macList.length >= 3, true);

  const macForm = macList.find((g) => g.category.includes("Formulário"));
  assert.ok(macForm);
  const macSave = macForm.shortcuts.find((s) => s.description.toLowerCase().includes("salvar"));
  assert.ok(macSave);
  assert.deepEqual(macSave.keys, ["⌘", "S"]);

  const winForm = winList.find((g) => g.category.includes("Formulário"));
  assert.ok(winForm);
  const winSave = winForm.shortcuts.find((s) => s.description.toLowerCase().includes("salvar"));
  assert.ok(winSave);
  assert.deepEqual(winSave.keys, ["Ctrl", "S"]);
});
