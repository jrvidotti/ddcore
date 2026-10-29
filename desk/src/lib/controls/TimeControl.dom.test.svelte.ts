import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import TimeControl from "./TimeControl.svelte";

vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { data: { lang: "en-GB", site: { timezone: "UTC" } } },
}));

// jsdom has no ResizeObserver; `anchored` only needs it to exist
globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-03-15T12:34:56Z"));
});
afterEach(() => vi.useRealTimers());

function setup(initial: string | null, readOnly = false) {
  const changes: any[] = [];
  const props = $state({
    field: { fieldname: "opens", fieldtype: "Time" } as any,
    value: initial as any,
    readOnly,
    onchange: (v: any) => { changes.push(v); props.value = v; },
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(TimeControl, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const popover = () => target.querySelector<HTMLElement>('[role="dialog"]');
  const flush = <T>(f: () => T) => { const r = f(); flushSync(); return r; };
  return {
    props, changes, target, input, popover,
    focus: () => flush(() => input.dispatchEvent(new FocusEvent("focus"))),
    blur: () => flush(() => input.dispatchEvent(new FocusEvent("blur"))),
    type: (s: string) => flush(() => { input.value = s; input.dispatchEvent(new Event("input", { bubbles: true })); }),
    key: (k: string, init: KeyboardEventInit = {}) => flush(() => input.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true, ...init }))),
    caret: (n: number) => input.setSelectionRange(n, n),
    click: (el: Element | null) => flush(() => (el as HTMLElement).click()),
    clockBtn: () => target.querySelector<HTMLButtonElement>(".cal-btn"),
    link: (label: string) => [...target.querySelectorAll<HTMLButtonElement>(".btn-link")].find((b) => b.textContent!.trim() === label) ?? null,
    slot: (label: string) => [...target.querySelectorAll<HTMLButtonElement>(".slot")].find((b) => b.textContent!.trim() === label) ?? null,
    done: () => { unmount(view); target.remove(); },
  };
}

describe("TimeControl", () => {
  it("is a text input that takes typed digits, not a native time input", () => {
    const t = setup("08:00:00");
    expect(t.input.type).toBe("text");
    expect(t.input.value).toBe("08:00");
    expect(t.input.placeholder).toBe("hh:mm");
    t.done();
  });

  it("completes once the fourth digit is in, and not before (#138)", () => {
    const t = setup(null);
    t.focus();
    t.type("080");
    expect(t.input.value).toBe("08:0");
    expect(t.changes).toEqual([]);
    t.type("0800");
    expect(t.input.value).toBe("08:00");
    expect(t.changes).toEqual(["08:00:00"]);
    t.done();
  });

  it("does not open the picker on a click in the input, so the keyboard stays there", () => {
    const t = setup(null);
    t.click(t.input);
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("takes the colon along with the digit before it on Backspace at the end", () => {
    const t = setup(null);
    t.focus();
    t.type("8");
    expect(t.input.value).toBe("08:");
    t.caret(3);
    t.key("Backspace");
    expect(t.input.value).toBe("0");
    t.done();
  });

  it("commits on blur, reverts what does not parse, and clears an emptied input", () => {
    const t = setup("08:00:00");
    t.focus();
    t.type("1");
    t.blur();
    expect(t.input.value).toBe("08:00");
    expect(t.changes).toEqual([]);
    t.focus();
    t.type("");
    t.blur();
    expect(t.changes).toEqual([null]);
    t.done();
  });

  it("keeps the seconds a value already has", () => {
    const t = setup("08:00:30");
    expect(t.input.value).toBe("08:00:30");
    t.focus();
    t.blur();
    expect(t.changes.every((c) => c === "08:00:30")).toBe(true);
    t.done();
  });

  it("steps the part under the caret with Up and Down", () => {
    const t = setup("08:00:00");
    t.focus();
    t.caret(1);
    t.key("ArrowUp");
    expect(t.changes.at(-1)).toBe("09:00:00");
    expect(t.input.value.slice(t.input.selectionStart!, t.input.selectionEnd!)).toBe("09");
    t.caret(4);
    t.key("ArrowDown");
    expect(t.changes.at(-1)).toBe("09:59:00");
    t.done();
  });

  it("fills an empty input with the site's current time on the first step", () => {
    const t = setup(null);
    t.focus();
    t.key("ArrowUp");
    expect(t.changes).toEqual(["12:34:00"]);
    t.done();
  });

  it("offers half-hour slots behind the clock icon, the value's one selected", () => {
    const t = setup("08:30:00");
    t.click(t.clockBtn());
    expect(t.popover()).not.toBeNull();
    expect(t.target.querySelectorAll(".slot")).toHaveLength(48);
    expect(t.slot("08:30")!.classList.contains("selected")).toBe(true);
    t.click(t.slot("17:00"));
    expect(t.changes).toEqual(["17:00:00"]);
    expect(t.input.value).toBe("17:00");
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("Now and Clear in the picker set the value", () => {
    const t = setup("08:00:00");
    t.click(t.clockBtn());
    t.click(t.link("Now"));
    expect(t.changes).toEqual(["12:34:00"]);
    t.click(t.clockBtn());
    t.click(t.link("Clear"));
    expect(t.changes).toEqual(["12:34:00", null]);
    t.done();
  });

  it("is inert when read-only", () => {
    const t = setup("08:00:00", true);
    t.key("ArrowUp");
    expect(t.changes).toEqual([]);
    expect(t.clockBtn()).toBeNull();
    t.done();
  });
});
