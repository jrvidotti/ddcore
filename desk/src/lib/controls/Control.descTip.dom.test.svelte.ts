import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(args[Number(i)])),
  boot: { data: { lang: "en", doctypes: {}, site: { currency: "USD", timezone: "UTC" } } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn(), confirm: vi.fn(), dialog: vi.fn() }));

// jsdom has no ResizeObserver; `anchored` only needs it to exist
globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;

import Control from "./Control.svelte";

const text = "Daily average: over the days of that weekday that had a visit.";
const select = { fieldname: "show", fieldtype: "Select", label: "Show As", options: ["Total", "Daily Average"], description: text };

function setup(field: any) {
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(Control, { target, props: { field, value: null, onchange: () => {} } });
  flushSync();
  const btn = () => target.querySelector<HTMLButtonElement>("button.info-btn");
  const tip = () => target.querySelector(".tip");
  const fire = (el: Element, e: Event) => { el.dispatchEvent(e); flushSync(); };
  return { target, btn, tip, fire, done: () => { unmount(view); target.remove(); } };
}

describe("descriptionPlacement", () => {
  it("draws the description under the input by default", () => {
    const t = setup(select);
    expect(t.target.querySelector(".desc")?.textContent).toBe(text);
    expect(t.btn()).toBe(null);
    t.done();
  });

  it("\"tooltip\" puts it behind an info icon beside the label, not in the label", () => {
    const t = setup({ ...select, descriptionPlacement: "tooltip" });
    expect(t.target.querySelector(".desc")).toBe(null);
    const btn = t.btn()!;
    expect(btn.getAttribute("aria-label")).toBe("About Show As");
    // the label names the select alone: the icon would otherwise join its accessible name
    expect(t.target.querySelector("label")!.contains(btn)).toBe(false);
    expect(t.target.querySelector("label")!.textContent).toBe("Show As");
    // read to a screen reader while the tip is closed
    expect(t.tip()).toBe(null);
    expect(document.getElementById(btn.getAttribute("aria-describedby")!)?.textContent).toBe(text);
    t.done();
  });

  it("opens on hover and on a keyboard focus, stays open on a click, and closes on Escape or a press outside", () => {
    const t = setup({ ...select, descriptionPlacement: "tooltip" });
    const btn = t.btn()!;
    t.fire(btn, new PointerEvent("pointerenter", { pointerType: "mouse" }));
    expect(t.tip()?.textContent).toBe(text);
    t.fire(btn, new PointerEvent("pointerleave", { pointerType: "mouse" }));
    expect(t.tip()).toBe(null);

    t.fire(btn, new FocusEvent("focus"));
    expect(t.tip()).not.toBe(null);
    t.fire(btn, new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(t.tip()).toBe(null);
    t.fire(btn, new FocusEvent("blur"));

    // a tap: no hover, and the focus that comes with the press does not count as a keyboard one
    t.fire(btn, new PointerEvent("pointerdown", { pointerType: "touch", bubbles: true }));
    t.fire(btn, new FocusEvent("focus"));
    expect(t.tip()).toBe(null);
    t.fire(btn, new MouseEvent("click", { bubbles: true }));
    expect(t.tip()).not.toBe(null);
    t.fire(document.body, new PointerEvent("pointerdown", { bubbles: true }));
    expect(t.tip()).toBe(null);
    t.done();
  });

  it("keeps it below when the label is hidden, as there is no label to sit beside", () => {
    const t = setup({ ...select, descriptionPlacement: "tooltip", hideLabel: true });
    expect(t.btn()).toBe(null);
    expect(t.target.querySelector(".desc")?.textContent).toBe(text);
    t.done();
  });

  it("works on a Check, beside its label", () => {
    const t = setup({ fieldname: "only_open", fieldtype: "Check", label: "Only open", description: "Leaves out closed stores", descriptionPlacement: "tooltip" });
    expect(t.btn()?.getAttribute("aria-label")).toBe("About Only open");
    expect(t.target.querySelector(".desc")).toBe(null);
    t.done();
  });
});
