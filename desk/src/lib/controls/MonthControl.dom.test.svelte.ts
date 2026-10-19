import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import MonthControl from "./MonthControl.svelte";

vi.mock("$lib/boot.svelte", () => ({
  __: (s: string) => s,
  boot: { data: { lang: "en-GB", site: { timezone: "UTC" } } },
}));

// jsdom has no ResizeObserver; `anchored` only needs it to exist
globalThis.ResizeObserver ??= class { observe() {} disconnect() {} } as any;

beforeEach(() => {
  vi.useFakeTimers({ toFake: ["Date"] });
  vi.setSystemTime(new Date("2026-03-15T12:00:00Z"));
});
afterEach(() => vi.useRealTimers());

function setup(initial: string | null, readOnly = false) {
  const changes: any[] = [];
  const props = $state({
    field: { fieldname: "period", fieldtype: "Month" } as any,
    value: initial as any,
    readOnly,
    onchange: (v: any) => { changes.push(v); props.value = v; },
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(MonthControl, { target, props });
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
    calBtn: () => target.querySelector<HTMLButtonElement>(".cal-btn"),
    link: (label: string) => [...target.querySelectorAll<HTMLButtonElement>(".btn-link")].find((b) => b.textContent!.trim() === label) ?? null,
    cell: (n: number) => target.querySelectorAll<HTMLButtonElement>(".month-cell")[n - 1],
    year: () => target.querySelector(".view-year")?.textContent,
    done: () => { unmount(view); target.remove(); },
  };
}

describe("MonthControl", () => {
  it("shows the value in the locale's order and masks what is typed", () => {
    const t = setup("2026-03-01");
    expect(t.input.value).toBe("03/2026");
    t.focus();
    t.type("");
    t.type("1");
    expect(t.input.value).toBe("1");
    t.type("3");
    expect(t.input.value).toBe("03/");
    t.type("03/20a");
    expect(t.input.value).toBe("03/20");
    t.done();
  });

  it("completes once the sixth digit is in, and not before", () => {
    const t = setup(null);
    t.focus();
    t.type("03202");
    expect(t.changes).toEqual([]);
    t.type("032026");
    expect(t.changes).toEqual(["2026-03-01"]);
    expect(t.input.value).toBe("03/2026");
    t.done();
  });

  it("takes the separator along with the digit before it on Backspace at the end", () => {
    const t = setup(null);
    t.focus();
    t.type("3");
    expect(t.input.value).toBe("03/");
    t.caret(3);
    t.key("Backspace");
    expect(t.input.value).toBe("0");
    t.done();
  });

  it("leaves Backspace alone away from a trailing separator", () => {
    const t = setup(null);
    t.focus();
    t.type("3");
    t.caret(1);
    const e = new KeyboardEvent("keydown", { key: "Backspace", bubbles: true, cancelable: true });
    t.input.dispatchEvent(e);
    expect(e.defaultPrevented).toBe(false);
    t.done();
  });

  it("commits on blur, reverts what does not parse, and clears an emptied input", () => {
    const t = setup("2026-03-01");
    t.focus();
    t.type("1");
    t.blur();
    expect(t.input.value).toBe("03/2026");
    expect(t.changes).toEqual([]);
    t.focus();
    t.type("");
    t.blur();
    expect(t.changes).toEqual([null]);
    expect(t.input.value).toBe("");
    t.done();
  });

  it("Enter commits, blurs and closes the picker", () => {
    const t = setup("2026-03-01");
    t.input.focus();
    t.click(t.input);
    expect(t.popover()).not.toBeNull();
    t.type("03/20");
    t.key("Enter");
    expect(t.popover()).toBeNull();
    expect(document.activeElement).not.toBe(t.input);
    expect(t.input.value).toBe("03/2026");
    t.done();
  });

  it("Escape closes an open picker and stops there; with none open it passes on", () => {
    const t = setup(null);
    const seen = vi.fn();
    window.addEventListener("keydown", seen);
    t.key("Escape");
    expect(seen).toHaveBeenCalledTimes(1);
    t.key("ArrowDown", { altKey: true });
    expect(t.popover()).not.toBeNull();
    t.key("Escape");
    expect(t.popover()).toBeNull();
    expect(seen).toHaveBeenCalledTimes(2);
    window.removeEventListener("keydown", seen);
    t.done();
  });

  it("Alt+Down opens the picker on the value's year, and again closes it", () => {
    const t = setup("2024-11-01");
    t.key("ArrowDown", { altKey: true });
    expect(t.popover()).not.toBeNull();
    expect(t.year()).toBe("2024");
    t.key("ArrowDown", { altKey: true });
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("opens on the current year when empty", () => {
    const t = setup(null);
    t.click(t.calBtn());
    expect(t.year()).toBe("2026");
    t.done();
  });

  it("steps the part under the caret with Up and Down", () => {
    const t = setup("2026-03-01");
    t.focus();
    t.caret(0);
    t.key("ArrowUp");
    expect(t.changes.at(-1)).toBe("2026-04-01");
    expect(t.input.value).toBe("04/2026");
    expect(t.input.value.slice(t.input.selectionStart!, t.input.selectionEnd!)).toBe("04");
    t.caret(5);
    t.key("ArrowDown");
    expect(t.changes.at(-1)).toBe("2025-04-01");
    t.done();
  });

  it("fills an empty input with this month on the first step", () => {
    const t = setup(null);
    t.focus();
    t.key("ArrowUp");
    expect(t.changes).toEqual(["2026-03-01"]);
    t.done();
  });

  it("does not step a read-only input, offers no picker, and ignores clicks", () => {
    const t = setup("2026-03-01", true);
    t.key("ArrowUp");
    t.click(t.input);
    expect(t.changes).toEqual([]);
    expect(t.calBtn()).toBeNull();
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("This month and Clear in the picker set the value, close it and return the focus", () => {
    const t = setup("2025-01-01");
    t.click(t.calBtn());
    t.click(t.link("This month"));
    expect(t.changes).toEqual(["2026-03-01"]);
    expect(t.popover()).toBeNull();
    expect(document.activeElement).toBe(t.input);
    t.click(t.calBtn());
    t.click(t.link("Clear"));
    expect(t.changes).toEqual(["2026-03-01", null]);
    expect(t.input.value).toBe("");
    expect(t.popover()).toBeNull();
    t.click(t.calBtn());
    expect(t.link("Clear")).toBeNull();
    t.done();
  });

  it("closes on a mousedown outside, not on one inside the picker", () => {
    const t = setup(null);
    t.click(t.calBtn());
    t.popover()!.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    flushSync();
    expect(t.popover()).not.toBeNull();
    document.body.dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    flushSync();
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("pages through the years and picks a month from the grid", () => {
    const t = setup("2026-03-01");
    t.click(t.calBtn());
    t.click(t.target.querySelector('[aria-label="Previous year"]'));
    expect(t.year()).toBe("2025");
    t.click(t.target.querySelector('[aria-label="Next year"]'));
    t.click(t.target.querySelector('[aria-label="Next year"]'));
    expect(t.year()).toBe("2027");
    t.click(t.cell(6));
    expect(t.changes).toEqual(["2027-06-01"]);
    expect(t.input.value).toBe("06/2027");
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("marks the selected month and this month", () => {
    const t = setup("2026-08-01");
    t.click(t.calBtn());
    expect(t.target.querySelector(".month-cell.selected")).toBe(t.cell(8));
    expect(t.target.querySelector(".month-cell.is-today")).toBe(t.cell(3));
    t.done();
  });
});
