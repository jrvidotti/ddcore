import { afterEach, describe, expect, it, vi } from "vitest";
import { dismiss } from "./floating";

const cleanups: (() => void)[] = [];
afterEach(() => cleanups.splice(0).forEach((fn) => fn()));

function setup(onclose: ((e: Event) => void) | null) {
  const node = document.createElement("div");
  const inside = document.createElement("button");
  const outside = document.createElement("button");
  node.append(inside);
  document.body.append(node, outside);
  const action = dismiss(node, onclose);
  cleanups.push(() => { action.destroy(); node.remove(); outside.remove(); });
  return { node, inside, outside, action };
}

const press = (el: Element) => el.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true }));
const escape = (el: Element) => el.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true }));

describe("dismiss", () => {
  it("closes on a pointer press outside the node, not on one inside it", () => {
    const onclose = vi.fn();
    const t = setup(onclose);
    press(t.inside);
    press(t.node);
    expect(onclose).not.toHaveBeenCalled();
    press(t.outside);
    expect(onclose).toHaveBeenCalledTimes(1);
  });

  it("closes on Escape and swallows it when it comes from inside the node", () => {
    const onclose = vi.fn();
    const t = setup(onclose);
    const seen = vi.fn();
    window.addEventListener("keydown", seen);
    cleanups.push(() => window.removeEventListener("keydown", seen));
    escape(t.inside);
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(seen).not.toHaveBeenCalled();
    escape(t.outside);
    expect(onclose).toHaveBeenCalledTimes(2);
    expect(seen).toHaveBeenCalledTimes(1);
  });

  it("ignores other keys", () => {
    const onclose = vi.fn();
    const t = setup(onclose);
    t.inside.dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    expect(onclose).not.toHaveBeenCalled();
  });

  it("does nothing, and lets Escape through, while the handler is null", () => {
    const t = setup(null);
    const seen = vi.fn();
    window.addEventListener("keydown", seen);
    cleanups.push(() => window.removeEventListener("keydown", seen));
    press(t.outside);
    escape(t.inside);
    expect(seen).toHaveBeenCalledTimes(1);
  });

  it("listens only while there is a handler", () => {
    const add = vi.spyOn(window, "addEventListener");
    const remove = vi.spyOn(window, "removeEventListener");
    const t = setup(null);
    expect(add).not.toHaveBeenCalled();
    t.action.update(vi.fn());
    expect(add).toHaveBeenCalledTimes(2);
    t.action.update(null);
    expect(remove).toHaveBeenCalledTimes(2);
    add.mockRestore();
    remove.mockRestore();
  });

  it("follows a new handler, and stops listening once destroyed", () => {
    const first = vi.fn();
    const second = vi.fn();
    const t = setup(first);
    t.action.update(second);
    press(t.outside);
    expect(first).not.toHaveBeenCalled();
    expect(second).toHaveBeenCalledTimes(1);
    t.action.destroy();
    press(t.outside);
    escape(t.inside);
    expect(second).toHaveBeenCalledTimes(1);
  });
});
