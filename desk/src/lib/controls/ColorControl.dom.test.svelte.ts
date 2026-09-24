import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import ColorControl from "./ColorControl.svelte";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));

// jsdom has no ResizeObserver; `anchored` only needs it to exist
globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;

function setup(initial: string | null, readOnly = false) {
  const props = $state({ value: initial as any, readOnly, onchange: (v: any) => (props.value = v) });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(ColorControl, { target, props });
  flushSync();
  const trigger = target.querySelector<HTMLButtonElement>("button.color-trigger")!;
  const popover = () => target.querySelector(".color-popover");
  const click = (el: Element) => { (el as HTMLElement).click(); flushSync(); };
  const button = (label: string) =>
    [...target.querySelectorAll("button")].find((b) => b.textContent === label || b.getAttribute("aria-label") === label)!;
  return { props, target, trigger, popover, click, button, done: () => { unmount(view); target.remove(); } };
}

describe("ColorControl", () => {
  it("shows the colour, not its hex", () => {
    const t = setup("#f97316");
    expect(t.trigger.textContent!.trim()).toBe("");
    expect(t.target.querySelector<HTMLElement>(".swatch")!.style.backgroundColor).toBe("rgb(249, 115, 22)");
    t.done();
  });

  it("opens on the basic grid and picks a colour from it", () => {
    const t = setup(null);
    t.click(t.trigger);
    expect(t.popover()).not.toBe(null);
    t.click(t.button("#2563eb"));
    expect(t.props.value).toBe("#2563eb");
    expect(t.popover()).toBe(null);
    t.done();
  });

  it("takes a typed hex under Advanced, and refuses what is not one", () => {
    const t = setup("#000000");
    t.click(t.trigger);
    t.click(t.button("Advanced"));
    const input = t.target.querySelector<HTMLInputElement>("input.hex")!;
    input.value = "ABC";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    flushSync();
    expect(t.props.value).toBe("#aabbcc");
    input.value = "nope";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    flushSync();
    input.dispatchEvent(new FocusEvent("blur"));
    flushSync();
    expect(t.props.value).toBe("#aabbcc");
    expect(input.value).toBe("#aabbcc");
    t.done();
  });

  it("sets a channel under Advanced", () => {
    const t = setup("#f97316");
    t.click(t.trigger);
    t.click(t.button("Advanced"));
    const [r, g, b] = t.target.querySelectorAll<HTMLInputElement>('input[type="number"]');
    expect([r.value, g.value, b.value]).toEqual(["249", "115", "22"]);
    b.value = "255";
    b.dispatchEvent(new Event("change", { bubbles: true }));
    flushSync();
    expect(t.props.value).toBe("#f973ff");
    expect(t.target.querySelector<HTMLInputElement>("input.hex")!.value).toBe("#f973ff");
    t.done();
  });

  it("clears", () => {
    const t = setup("#16a34a");
    t.click(t.trigger);
    t.click(t.button("Clear"));
    expect(t.props.value).toBe(null);
    t.done();
  });

  it("does not open when read-only", () => {
    const t = setup("#16a34a", true);
    t.click(t.trigger);
    expect(t.popover()).toBe(null);
    t.done();
  });
});
