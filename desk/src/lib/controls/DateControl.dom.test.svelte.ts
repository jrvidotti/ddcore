import { flushSync, mount, unmount } from "svelte";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import DateControl from "./DateControl.svelte";

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

function setup(initial: string | null, readOnly = false, min: string | null = null) {
  const changes: any[] = [];
  const refused: boolean[] = [];
  const props = $state({
    min,
    onrefused: (r: boolean) => refused.push(r),
    field: { fieldname: "due", fieldtype: "Date" } as any,
    value: initial as any,
    readOnly,
    onchange: (v: any) => { changes.push(v); props.value = v; },
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(DateControl, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const popover = () => target.querySelector<HTMLElement>('[role="dialog"]');
  const flush = <T>(f: () => T) => { const r = f(); flushSync(); return r; };
  return {
    props, changes, refused, target, input, popover,
    focus: () => flush(() => input.dispatchEvent(new FocusEvent("focus"))),
    blur: () => flush(() => input.dispatchEvent(new FocusEvent("blur"))),
    type: (s: string) => flush(() => { input.value = s; input.dispatchEvent(new Event("input", { bubbles: true })); }),
    key: (k: string, init: KeyboardEventInit = {}) => flush(() => input.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true, ...init }))),
    caret: (n: number) => input.setSelectionRange(n, n),
    click: (el: Element | null) => flush(() => (el as HTMLElement).click()),
    calBtn: () => target.querySelector<HTMLButtonElement>(".cal-btn"),
    link: (label: string) => [...target.querySelectorAll<HTMLButtonElement>(".btn-link")].find((b) => b.textContent!.trim() === label) ?? null,
    title: () => target.querySelector(".view-title")?.textContent,
    done: () => { unmount(view); target.remove(); },
  };
}

describe("DateControl", () => {
  it("shows the value in the locale's order and masks what is typed", () => {
    const t = setup("2026-03-15");
    expect(t.input.value).toBe("15/03/2026");
    t.focus();
    t.type("");
    t.type("1");
    expect(t.input.value).toBe("1");
    t.type("15");
    expect(t.input.value).toBe("15/");
    t.type("15/0a3");
    expect(t.input.value).toBe("15/03/");
    t.done();
  });

  it("completes once the eighth digit is in, and not before", () => {
    const t = setup(null);
    t.focus();
    t.type("1503202");
    expect(t.changes).toEqual([]);
    t.type("15032026");
    expect(t.changes).toEqual(["2026-03-15"]);
    expect(t.input.value).toBe("15/03/2026");
    t.done();
  });

  it("takes the separator along with the digit before it on Backspace at the end", () => {
    const t = setup(null);
    t.focus();
    t.type("15");
    expect(t.input.value).toBe("15/");
    t.caret(3);
    t.key("Backspace");
    expect(t.input.value).toBe("1");
    t.done();
  });

  it("leaves Backspace alone away from a trailing separator", () => {
    const t = setup(null);
    t.focus();
    t.type("15");
    t.caret(1);
    const e = new KeyboardEvent("keydown", { key: "Backspace", bubbles: true, cancelable: true });
    t.input.dispatchEvent(e);
    expect(e.defaultPrevented).toBe(false);
    t.done();
  });

  it("commits on blur, reverts what does not parse, and clears an emptied input", () => {
    const t = setup("2026-03-15");
    t.focus();
    t.type("1");
    t.blur();
    expect(t.input.value).toBe("15/03/2026");
    expect(t.changes).toEqual([]);
    t.focus();
    t.type("");
    t.blur();
    expect(t.changes).toEqual([null]);
    expect(t.input.value).toBe("");
    t.done();
  });

  it("Enter commits, blurs and closes the picker", () => {
    const t = setup("2026-03-15");
    t.input.focus();
    t.click(t.input);
    expect(t.popover()).not.toBeNull();
    t.type("150320");
    t.key("Enter");
    expect(t.popover()).toBeNull();
    expect(document.activeElement).not.toBe(t.input);
    expect(t.input.value).toBe("15/03/2026");
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

  it("Alt+Down opens the picker on the value's month, and again closes it", () => {
    const t = setup("2025-11-02");
    t.key("ArrowDown", { altKey: true });
    expect(t.popover()).not.toBeNull();
    expect(t.title()).toBe("November 2025");
    t.key("ArrowDown", { altKey: true });
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("opens on the current month when empty", () => {
    const t = setup(null);
    t.click(t.calBtn());
    expect(t.title()).toBe("March 2026");
    t.done();
  });

  it("steps the part under the caret with Up and Down", () => {
    const t = setup("2026-03-15");
    t.focus();
    t.caret(0);
    t.key("ArrowUp");
    expect(t.changes.at(-1)).toBe("2026-03-16");
    expect(t.input.value).toBe("16/03/2026");
    expect(t.input.value.slice(t.input.selectionStart!, t.input.selectionEnd!)).toBe("16");
    t.caret(3);
    t.key("ArrowDown");
    expect(t.changes.at(-1)).toBe("2026-02-16");
    t.caret(7);
    t.key("ArrowUp");
    expect(t.changes.at(-1)).toBe("2027-02-16");
    t.done();
  });

  it("fills an empty input with today on the first step", () => {
    const t = setup(null);
    t.focus();
    t.key("ArrowUp");
    expect(t.changes).toEqual(["2026-03-15"]);
    t.done();
  });

  it("does not step a read-only input, offers no picker, and ignores clicks", () => {
    const t = setup("2026-03-15", true);
    t.key("ArrowUp");
    t.click(t.input);
    expect(t.changes).toEqual([]);
    expect(t.calBtn()).toBeNull();
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("Today and Clear in the picker set the value, close it and return the focus", () => {
    const t = setup("2026-01-02");
    t.click(t.calBtn());
    t.click(t.link("Today"));
    expect(t.changes).toEqual(["2026-03-15"]);
    expect(t.popover()).toBeNull();
    expect(document.activeElement).toBe(t.input);
    t.click(t.calBtn());
    t.click(t.link("Clear"));
    expect(t.changes).toEqual(["2026-03-15", null]);
    expect(t.input.value).toBe("");
    expect(t.popover()).toBeNull();
    t.click(t.calBtn());
    expect(t.link("Clear")).toBeNull();
    t.done();
  });

  it("closes on a pointer press outside, not on one inside the picker", () => {
    const t = setup(null);
    t.click(t.calBtn());
    t.popover()!.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    flushSync();
    expect(t.popover()).not.toBeNull();
    document.body.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
    flushSync();
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("pages through the months and picks a day from the grid", () => {
    const t = setup("2026-03-15");
    t.click(t.calBtn());
    t.click(t.target.querySelector('[aria-label="Previous month"]'));
    expect(t.title()).toBe("February 2026");
    t.click(t.target.querySelector('[aria-label="Next month"]'));
    t.click(t.target.querySelector('[aria-label="Next month"]'));
    expect(t.title()).toBe("April 2026");
    const day = [...t.target.querySelectorAll<HTMLButtonElement>(".day-cell:not(.other-month)")].find((b) => b.textContent!.trim() === "20");
    t.click(day!);
    expect(t.changes).toEqual(["2026-04-20"]);
    expect(t.input.value).toBe("20/04/2026");
    expect(t.popover()).toBeNull();
    t.done();
  });

  it("marks the selected day and today", () => {
    const t = setup("2026-03-20");
    t.click(t.calBtn());
    expect(t.target.querySelector(".day-cell.selected")?.textContent?.trim()).toBe("20");
    expect(t.target.querySelector(".day-cell.is-today")?.textContent?.trim()).toBe("15");
    t.done();
  });

  describe("with a minimum (minDateField)", () => {
    const day = (t: ReturnType<typeof setup>, n: string) =>
      [...t.target.querySelectorAll<HTMLButtonElement>(".day-cell:not(.other-month)")].find((b) => b.textContent!.trim() === n)!;

    it("opens on the minimum's month when empty, with the days before it disabled", () => {
      const t = setup(null, false, "2026-05-10");
      t.click(t.calBtn());
      expect(t.title()).toBe("May 2026");
      expect(day(t, "9").disabled).toBe(true);
      expect(day(t, "10").disabled).toBe(false);
      t.click(day(t, "9"));
      expect(t.changes).toEqual([]);
      t.click(day(t, "12"));
      expect(t.changes).toEqual(["2026-05-12"]);
      t.done();
    });

    it("disables Today when today is before the minimum", () => {
      const t = setup(null, false, "2026-05-10");
      t.click(t.calBtn());
      expect(t.link("Today")!.disabled).toBe(true);
      t.done();
    });

    it("opens on its own value's month when it has one", () => {
      const t = setup("2026-07-01", false, "2026-05-10");
      t.click(t.calBtn());
      expect(t.title()).toBe("July 2026");
      t.done();
    });

    it("refuses an earlier day typed, and puts the value back on blur", () => {
      const t = setup("2026-05-20", false, "2026-05-10");
      t.focus();
      t.type("09052026");
      expect(t.changes).toEqual([]);
      expect(t.refused.at(-1)).toBe(true);
      t.blur();
      expect(t.changes).toEqual([]);
      expect(t.input.value).toBe("20/05/2026");
      t.focus();
      t.type("11052026");
      expect(t.changes).toEqual(["2026-05-11"]);
      expect(t.refused.at(-1)).toBe(false);
      t.done();
    });

    it("does not step below the minimum with the arrow keys", () => {
      const t = setup("2026-05-10", false, "2026-05-10");
      t.focus();
      t.caret(0);
      t.key("ArrowDown");
      expect(t.changes).toEqual([]);
      t.key("ArrowUp");
      expect(t.changes).toEqual(["2026-05-11"]);
      t.done();
    });

    it("changes nothing without a minimum", () => {
      const t = setup(null);
      t.click(t.calBtn());
      expect(t.title()).toBe("March 2026");
      expect(day(t, "1").disabled).toBe(false);
      t.done();
    });
  });
});
