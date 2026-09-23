// @vitest-environment jsdom
import { afterEach, describe, expect, it } from "vitest";
import { focusTrap, initialFocus } from "./focus-trap";

function modal(html: string) {
  const opener = document.createElement("button");
  opener.textContent = "open";
  document.body.append(opener);
  opener.focus();
  const node = document.createElement("div");
  node.innerHTML = html;
  document.body.append(node);
  return { opener, node };
}

const tab = (shiftKey = false) => new KeyboardEvent("keydown", { key: "Tab", shiftKey, bubbles: true, cancelable: true });

afterEach(() => { document.body.innerHTML = ""; });

describe("focus trap", () => {
  it("takes the focus from the button that opened the modal, onto its first field", () => {
    const { node } = modal(`<button id="x">×</button><input id="a"><button id="ok">OK</button>`);
    focusTrap(node);
    expect(document.activeElement?.id).toBe("a");
  });

  it("focuses the modal itself when it has no field, not its first button", () => {
    const { node } = modal(`<button id="x">×</button><button id="ok">OK</button>`);
    focusTrap(node);
    expect(document.activeElement).toBe(node);
    expect(initialFocus(node)).toBe(node);
  });

  it("skips a read-only field", () => {
    const { node } = modal(`<input id="ro" readonly><input id="a">`);
    expect(initialFocus(node).id).toBe("a");
  });

  it("does not open a Link field's options list by focusing it", () => {
    const { node } = modal(`<input id="l" data-fieldtype="Link"><input id="a">`);
    expect(initialFocus(node)).toBe(node);
  });

  it("keeps Tab and Shift+Tab inside the modal", () => {
    const { node } = modal(`<input id="a"><button id="ok">OK</button>`);
    focusTrap(node);
    (node.querySelector("#ok") as HTMLElement).focus();
    const fwd = tab();
    document.activeElement!.dispatchEvent(fwd);
    expect(fwd.defaultPrevented).toBe(true);
    expect(document.activeElement?.id).toBe("a");
    const back = tab(true);
    document.activeElement!.dispatchEvent(back);
    expect(document.activeElement?.id).toBe("ok");
  });

  it("gives the focus back to the opener when the modal closes", () => {
    const { opener, node } = modal(`<input id="a">`);
    const trap = focusTrap(node);
    node.remove();
    trap.destroy();
    expect(document.activeElement).toBe(opener);
  });
});
