import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { data: { lang: "pt-BR", doctypes: {}, site: { currency: "BRL", timezone: "UTC" } } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn(), confirm: vi.fn(), dialog: vi.fn() }));

import Control from "./Control.svelte";

function setup(field: any, initial: number | null) {
  const changes: any[] = [];
  const props = $state({ field, value: initial as any, onchange: (v: any) => { changes.push(v); props.value = v; }, doc: {} });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(Control, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const type = (s: string) => {
    input.dispatchEvent(new FocusEvent("focus"));
    input.value = s;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new FocusEvent("blur"));
    flushSync();
  };
  return { props, changes, input, type, done: () => { unmount(view); target.remove(); } };
}

describe("a number control with a display scale", () => {
  const field = { fieldname: "planned", fieldtype: "Currency", precision: 0, label: "Planned", displayScale: 1000, displayPrecision: 1 };

  it("shows the value in the scale and stores what is typed out of it", () => {
    const t = setup(field, 41828);
    expect(t.input.value).toBe("41,8");
    t.type("120");
    expect(t.changes).toEqual([120000]);
    expect(t.input.value).toBe("120,0");
    t.type("1,65");
    expect(t.changes.at(-1)).toBe(1700);
    t.type("");
    expect(t.changes.at(-1)).toBeNull();
    t.done();
  });

  it("follows the scale when a script changes it, without touching the value", () => {
    const t = setup(field, 41828);
    t.props.field = { ...field, displayScale: 1, displayPrecision: 0 };
    flushSync();
    expect(t.input.value).toBe("41.828");
    expect(t.changes).toEqual([]);
    t.done();
  });

  it("a field only passed through commits nothing, and shows what a script changes meanwhile", () => {
    const t = setup(field, 41828);
    t.input.dispatchEvent(new FocusEvent("focus"));
    flushSync();
    // the value changes while the field has the focus and nothing was typed in it
    t.props.value = 50000;
    flushSync();
    expect(t.input.value).toBe("50,0");
    t.input.dispatchEvent(new FocusEvent("blur"));
    flushSync();
    // 41,8 was never put back, and 50,0 was not committed as a rounded 50000 either
    expect(t.changes).toEqual([]);
    // once something is typed, a change from outside does not take the text away
    t.input.dispatchEvent(new FocusEvent("focus"));
    t.input.value = "7";
    t.input.dispatchEvent(new Event("input", { bubbles: true }));
    t.props.value = 60000;
    flushSync();
    expect(t.input.value).toBe("7");
    t.input.dispatchEvent(new FocusEvent("blur"));
    flushSync();
    expect(t.changes).toEqual([7000]);
    t.done();
  });

  it("a field without a scale is as before", () => {
    const t = setup({ fieldname: "planned", fieldtype: "Currency", precision: 0, label: "Planned" }, 41828);
    expect(t.input.value).toBe("41.828");
    t.type("500");
    expect(t.changes).toEqual([500]);
    t.done();
  });
});
