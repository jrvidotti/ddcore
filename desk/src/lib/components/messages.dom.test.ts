import { flushSync, mount, unmount } from "svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: null, ready: false } }));
vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "" }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

import { confirm, escapeHtml, toast, ui } from "$lib/ui.svelte";
import Toasts from "./Toasts.svelte";
import Dialogs from "./Dialogs.svelte";

// #170: what a person typed into a document reaches these messages, so they are text
const payload = '<img src=x onerror="window.__pwned = true">';

let cleanup: (() => void)[] = [];
afterEach(() => {
  for (const f of cleanup) f();
  cleanup = [];
  ui.toasts.splice(0);
  ui.dialogs.splice(0);
});

function render(component: typeof Toasts | typeof Dialogs) {
  const target = document.createElement("div");
  document.body.appendChild(target);
  const c = mount(component, { target });
  cleanup.push(() => { unmount(c); target.remove(); });
  return target;
}

describe("toast message", () => {
  it("shows markup in a message as text", () => {
    const target = render(Toasts);
    toast(`CREF ${payload} copied`);
    flushSync();
    expect(target.querySelector("img")).toBeNull();
    expect(target.textContent).toContain(payload);
  });

  it("renders markup when the caller opts in", () => {
    const target = render(Toasts);
    toast(`<b>${escapeHtml(payload)}</b>`, { html: true });
    flushSync();
    expect(target.querySelector("b")?.textContent).toBe(payload);
    expect(target.querySelector("img")).toBeNull();
  });
});

describe("confirm message", () => {
  it("shows markup in a message as text", () => {
    const target = render(Dialogs);
    void confirm(`Did the lookup show ${payload}?`);
    flushSync();
    expect(target.querySelector(".message img")).toBeNull();
    expect(target.querySelector(".message")?.textContent).toBe(`Did the lookup show ${payload}?`);
  });

  it("renders markup when the caller opts in", () => {
    const target = render(Dialogs);
    void confirm("Delete <b>TASK-1</b>?", undefined, { html: true });
    flushSync();
    expect(target.querySelector(".message b")?.textContent).toBe("TASK-1");
  });
});
