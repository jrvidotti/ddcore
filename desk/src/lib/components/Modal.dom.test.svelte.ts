import { createRawSnippet, flushSync, mount, unmount } from "svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));

import Modal from "./Modal.svelte";

const body = createRawSnippet(() => ({ render: () => `<div class="body"><input id="in" /></div>` }));
let view: ReturnType<typeof mount> | undefined;

function open(props: Record<string, unknown> = {}) {
  const onclose = vi.fn();
  view = mount(Modal, { target: document.body, props: { title: "Share document", icon: "share-2", onclose, children: body, ...props } });
  flushSync();
  return { onclose, bg: document.querySelector<HTMLElement>(".modal-bg")! };
}

afterEach(() => {
  if (view) unmount(view);
  view = undefined;
  document.body.innerHTML = "";
});

describe("Modal", () => {
  it("is a labelled modal dialog titled by its heading, and takes the focus", () => {
    const { bg } = open({ size: "sm" });
    expect(bg.getAttribute("role")).toBe("dialog");
    expect(bg.getAttribute("aria-modal")).toBe("true");
    expect(document.getElementById(bg.getAttribute("aria-labelledby")!)?.textContent?.trim()).toBe("Share document");
    expect(bg.querySelector(".modal")?.classList.contains("sm")).toBe(true);
    expect(bg.contains(document.activeElement)).toBe(true);
  });

  it("closes from the close button, Escape and a click on the backdrop, but not a click inside", () => {
    const { bg, onclose } = open();
    bg.querySelector<HTMLButtonElement>("button[aria-label=Close]")!.click();
    bg.querySelector<HTMLElement>(".modal")!.click();
    expect(onclose).toHaveBeenCalledTimes(1);
    bg.click();
    expect(onclose).toHaveBeenCalledTimes(2);
    bg.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    expect(onclose).toHaveBeenCalledTimes(3);
  });

  it("keeps Escape from reaching the page behind it", () => {
    const { bg } = open();
    const behind = vi.fn();
    window.addEventListener("keydown", behind);
    bg.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    window.removeEventListener("keydown", behind);
    expect(behind).not.toHaveBeenCalled();
  });

  it("leaves an Escape that a control inside already used, such as one closing its list", () => {
    const { bg, onclose } = open();
    const esc = new KeyboardEvent("keydown", { key: "Escape", bubbles: true, cancelable: true });
    bg.querySelector<HTMLElement>("#in")!.addEventListener("keydown", (e) => e.preventDefault(), { once: true });
    bg.querySelector<HTMLElement>("#in")!.dispatchEvent(esc);
    expect(onclose).not.toHaveBeenCalled();
  });

  it("ignores Escape and the backdrop and disables the close button while not closable", () => {
    const { bg, onclose } = open({ closable: false });
    bg.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape", bubbles: true }));
    bg.click();
    expect(onclose).not.toHaveBeenCalled();
    expect(bg.querySelector<HTMLButtonElement>("button[aria-label=Close]")!.disabled).toBe(true);
  });

  it("wraps Tab from the last control back to the first", () => {
    const { bg } = open();
    const input = bg.querySelector<HTMLElement>("#in")!;
    input.focus();
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Tab", bubbles: true, cancelable: true }));
    expect(document.activeElement).toBe(bg.querySelector("button[aria-label=Close]"));
  });
});
