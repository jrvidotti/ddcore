import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import type { Meta } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), confirm: vi.fn(), dialog: vi.fn(), ui: { busy: 0 } }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "" }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

import { FormController } from "$lib/form.svelte";
import { showError } from "$lib/ui.svelte";
import Grid from "./Grid.svelte";
import FieldButtons from "./FieldButtons.svelte";

const childMeta = { name: "Tool Row", fields: [{ fieldname: "username", fieldtype: "Data", label: "Username", inListView: true, readOnly: true }] };
const meta = {
  doctype: {
    name: "Tool", label: "Tool", formApps: [],
    fields: [{ fieldname: "users", fieldtype: "Table", label: "Users", options: "Tool Row", gridSelect: true }],
  },
  children: { "Tool Row": childMeta },
  permissions: { read: true, write: true },
} as unknown as Meta;

function render(rows: any[] = []) {
  const frm = new FormController(meta, { doctype: "Tool", id: "Tool", users: rows });
  frm.addFieldButton("users", { icon: "refresh-cw", label: "Reload", onClick: () => {} });
  const target = document.createElement("div");
  document.body.appendChild(target);
  const view = mount(Grid, { target, props: { get frm() { return frm; }, get field() { return frm.field("users")!; }, childMeta: childMeta as any } });
  flushSync();
  return { frm, target, done: () => { unmount(view); target.remove(); } };
}

describe("Grid loading", () => {
  it("says Loading instead of No rows while a script fetches the rows, and No rows once it is done", () => {
    const { frm, target, done } = render();
    expect(target.querySelector("tbody")?.textContent).toContain("No rows");
    frm.setDfProperty("users", "loading", true);
    flushSync();
    expect(target.querySelector("tbody")?.textContent).toContain("Loading...");
    expect(target.querySelector("tbody")?.textContent).not.toContain("No rows");
    expect(target.querySelector("table")?.getAttribute("aria-busy")).toBe("true");
    expect(target.querySelector<HTMLButtonElement>("button[aria-label=Reload]")?.disabled).toBe(true);
    expect(target.querySelector<HTMLButtonElement>("button.btn.sm:not(.icon)")?.disabled).toBe(true); // Add row
    frm.setDfProperty("users", "loading", false);
    flushSync();
    expect(target.querySelector("tbody")?.textContent).toContain("No rows");
    expect(target.querySelector("table")?.hasAttribute("aria-busy")).toBe(false);
    expect(target.querySelector<HTMLButtonElement>("button[aria-label=Reload]")?.disabled).toBe(false);
    done();
  });

  it("keeps the rows on screen, dimmed, during a reload", () => {
    const { frm, target, done } = render([{ username: "ana" }]);
    frm.setDfProperty("users", "loading", true);
    flushSync();
    expect(target.querySelector("table")?.classList.contains("loading")).toBe(true);
    expect(target.querySelector("tbody")?.textContent).toContain("ana");
    expect(target.querySelector("tbody")?.textContent).not.toContain("Loading...");
    done();
  });
});

describe("FieldButtons", () => {
  it("keeps a button disabled, with a spinner, until the promise its onClick returns settles", async () => {
    let release!: () => void;
    const onClick = vi.fn(() => new Promise<void>((r) => (release = r)));
    const target = document.createElement("div");
    const view = mount(FieldButtons, { target, props: { buttons: [{ icon: "refresh-cw", label: "Reload", onClick }], small: true } });
    flushSync();
    const btn = target.querySelector<HTMLButtonElement>("button")!;
    btn.click();
    flushSync();
    expect(onClick).toHaveBeenCalledTimes(1);
    expect(btn.disabled).toBe(true);
    expect(btn.getAttribute("aria-busy")).toBe("true");
    expect(btn.querySelector(".btn-spin")).not.toBeNull();
    release();
    await Promise.resolve(); await Promise.resolve();
    flushSync();
    expect(btn.disabled).toBe(false);
    expect(btn.querySelector(".btn-spin")).toBeNull();
    unmount(view);
  });

  it("shows the error a failing onClick throws", async () => {
    const target = document.createElement("div");
    const view = mount(FieldButtons, { target, props: { buttons: [{ label: "Go", onClick: async () => { throw new Error("refused"); } }] } });
    flushSync();
    target.querySelector<HTMLButtonElement>("button")!.click();
    await Promise.resolve(); await Promise.resolve();
    flushSync();
    expect(showError).toHaveBeenCalled();
    expect(target.querySelector<HTMLButtonElement>("button")!.disabled).toBe(false);
    unmount(view);
  });
});
