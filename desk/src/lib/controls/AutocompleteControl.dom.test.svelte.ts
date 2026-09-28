import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it } from "vitest";
import AutocompleteControl from "./AutocompleteControl.svelte";
import type { Field } from "$lib/meta";

// jsdom has no ResizeObserver; `anchored` only needs it to exist
globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;

function setup(initial: string | null, options: any = ["Red", "Blue", "Rosé"]) {
  const changes: any[] = [];
  const props = $state({
    field: { fieldname: "tag", fieldtype: "Autocomplete", label: "Tag", options } as Field,
    value: initial as any,
    id: "f-tag",
    onchange: (v: any) => { changes.push(v); props.value = v; },
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(AutocompleteControl, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const shown = () => [...target.querySelectorAll('[role="option"]')].map((o) => o.textContent!.trim());
  const active = () => target.querySelector('[role="option"].active')?.textContent?.trim();
  const focus = () => { input.dispatchEvent(new FocusEvent("focus")); flushSync(); };
  const type = (s: string) => { input.value = s; input.dispatchEvent(new Event("input", { bubbles: true })); flushSync(); };
  const key = (k: string) => { input.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true })); flushSync(); };
  const blur = () => { input.dispatchEvent(new FocusEvent("blur")); flushSync(); };
  return { props, changes, target, input, shown, active, focus, type, key, blur, done: () => { unmount(view); target.remove(); } };
}

describe("AutocompleteControl", () => {
  it("shows the stored value and lists the suggestions on focus", () => {
    const t = setup("Blue");
    expect(t.input.value).toBe("Blue");
    expect(t.shown()).toEqual([]);
    t.focus();
    expect(t.shown()).toEqual(["Red", "Blue", "Rosé"]);
    t.done();
  });

  it("filters as the user types, ignoring accents", () => {
    const t = setup(null);
    t.focus();
    t.type("rose");
    expect(t.shown()).toEqual(["Rosé"]);
    t.type("r");
    expect(t.shown()).toEqual(["Red", "Rosé"]);
    t.done();
  });

  it("picks with the arrows and Enter", () => {
    const t = setup(null);
    t.focus();
    t.type("r");
    t.key("ArrowDown");
    t.key("ArrowDown");
    expect(t.active()).toBe("Rosé");
    t.key("Enter");
    expect(t.props.value).toBe("Rosé");
    expect(t.input.value).toBe("Rosé");
    expect(t.shown()).toEqual([]);
    t.done();
  });

  it("keeps the typed text when nothing is highlighted", () => {
    const t = setup(null);
    t.focus();
    t.type("Re");
    t.key("Enter");
    expect(t.props.value).toBe("Re");
    t.done();
  });

  it("Escape closes the list and changes nothing", () => {
    const t = setup("Blue");
    t.focus();
    t.key("Escape");
    expect(t.shown()).toEqual([]);
    expect(t.changes).toEqual([]);
    t.done();
  });

  it("commits free text on blur, trimmed, and a blank as null", () => {
    const t = setup(null);
    t.focus();
    t.type("  Green  ");
    expect(t.changes).toEqual([]);
    t.blur();
    expect(t.changes).toEqual(["Green"]);
    expect(t.input.value).toBe("Green");
    t.focus();
    t.type("  ");
    t.blur();
    expect(t.props.value).toBe(null);
    t.done();
  });

  it("does not call onchange when blur leaves the value as it was", () => {
    const t = setup("Blue");
    t.focus();
    t.blur();
    expect(t.changes).toEqual([]);
    t.done();
  });

  it("picks a suggestion with the mouse", () => {
    const t = setup(null);
    t.focus();
    const opt = [...t.target.querySelectorAll('[role="option"]')].find((o) => o.textContent!.trim() === "Blue")!;
    opt.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, cancelable: true }));
    flushSync();
    expect(t.props.value).toBe("Blue");
    t.done();
  });

  it("follows options replaced at runtime, as frm.setDfProperty does", () => {
    const t = setup(null, "Red\nBlue");
    t.focus();
    expect(t.shown()).toEqual(["Red", "Blue"]);
    t.props.field = { ...t.props.field, options: ["Amber", "Azure"] };
    flushSync();
    expect(t.shown()).toEqual(["Amber", "Azure"]);
    t.done();
  });
});
