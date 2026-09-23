import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import DurationControl from "./DurationControl.svelte";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));

// a .svelte.ts file, so the props can be $state and the control sees its own changes
function setup(initial: number | null) {
  const props = $state({ field: { fieldtype: "Duration" }, value: initial as any, onchange: (v: any) => (props.value = v) });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(DurationControl, { target, props });
  flushSync();
  const input = target.querySelector("input")!;
  const key = (k: string, init: KeyboardEventInit = {}) => {
    input.dispatchEvent(new KeyboardEvent("keydown", { key: k, bubbles: true, cancelable: true, ...init }));
    flushSync();
  };
  const selected = () => input.value.slice(input.selectionStart!, input.selectionEnd!);
  return { props, input, key, selected, done: () => { unmount(view); target.remove(); } };
}

describe("DurationControl", () => {
  it("steps the selected unit with up and down, and moves with left and right", () => {
    const t = setup(9000);
    expect(t.input.value).toBe("00d 02h 30m 00s");
    t.input.focus();
    t.key("ArrowRight");
    t.key("ArrowRight");
    expect(t.selected()).toBe("30");
    t.key("ArrowUp");
    expect(t.props.value).toBe(9060);
    expect(t.input.value).toBe("00d 02h 31m 00s");
    expect(t.selected()).toBe("31");
    t.key("ArrowLeft");
    t.key("ArrowDown");
    expect(t.props.value).toBe(5460);
    t.key("ArrowDown", { shiftKey: true });
    expect(t.props.value).toBe(0);
    t.done();
  });

  it("fills a unit from typed digits and moves on when it is full", () => {
    const t = setup(null);
    expect(t.input.value).toBe("");
    t.input.focus();
    t.key("ArrowRight");
    t.key("1");
    t.key("5");
    expect(t.props.value).toBe(15 * 3600);
    expect(t.selected()).toBe("00");
    t.key("Backspace");
    t.key("ArrowLeft");
    t.key("Backspace");
    expect(t.props.value).toBe(0);
    t.key("Backspace");
    expect(t.props.value).toBe(null);
    expect(t.input.value).toBe("");
    t.done();
  });
});
