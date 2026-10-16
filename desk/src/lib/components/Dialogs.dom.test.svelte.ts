import { flushSync, mount, tick, unmount } from "svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: { doctypes: {} } } }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
const shown: any[] = [];
vi.mock("$lib/ui.svelte", async (orig) => ({ ...(await orig<any>()), showError: (e: any) => shown.push(e) }));

import Dialogs from "./Dialogs.svelte";
import { dialog, ui } from "$lib/ui.svelte";

function setup(spec: any) {
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(Dialogs, { target });
  const d = dialog(spec);
  d.show();
  flushSync();
  const buttons = () => [...target.querySelectorAll<HTMLButtonElement>(".foot button")];
  const labels = () => buttons().map((b) => b.textContent?.trim());
  const button = (label: string) => buttons().find((b) => b.textContent?.trim() === label)!;
  return { target, d, buttons, labels, button, done: () => { unmount(view); target.remove(); } };
}

afterEach(() => { ui.dialogs.splice(0); shown.length = 0; });

describe("dialog actions", () => {
  const fields = [{ fieldname: "link", fieldtype: "Data", label: "Payment Link", reqd: true }];

  it("renders the actions in order, between Cancel and the primary button, with their icons", () => {
    const t = setup({
      title: "Payment Link", fields, primaryLabel: "Copy Link", primaryAction: vi.fn(),
      actions: [{ label: "Send by WhatsApp", icon: "message-circle", action: vi.fn() }, { label: "Send by Email", action: vi.fn() }],
    });
    expect(t.labels()).toEqual(["Cancel", "Send by WhatsApp", "Send by Email", "Copy Link"]);
    expect(t.button("Send by WhatsApp").querySelector("svg")).not.toBeNull();
    expect(t.button("Send by Email").querySelector("svg")).toBeNull();
    expect(t.button("Send by WhatsApp").classList.contains("primary")).toBe(false);
    expect(t.button("Send by WhatsApp").classList.contains("danger")).toBe(false);
    t.done();
  });

  it("runs an action with the values and the dialog, leaves it open, and skips the reqd check", async () => {
    const action = vi.fn();
    const t = setup({ title: "Payment Link", fields, values: { link: "" }, actions: [{ label: "Send", action }] });
    t.d.setValue("link", "");
    t.button("Send").click();
    await tick();
    expect(action).toHaveBeenCalledOnce();
    expect(action.mock.calls[0][0]).toEqual({ link: "" });
    expect(action.mock.calls[0][1].id).toBe(t.d.id);
    expect(ui.dialogs).toHaveLength(1);
    expect(shown).toEqual([]);
    t.done();
  });

  it("disables every action button while an async action runs, and shows its error", async () => {
    let fail!: (e: Error) => void;
    const t = setup({
      title: "Payment Link", fields, primaryLabel: "Copy Link", primaryAction: vi.fn(), dangerLabel: "Revoke", dangerAction: vi.fn(),
      actions: [{ label: "Send", action: () => new Promise((_, reject) => (fail = reject)) }],
    });
    t.button("Send").click();
    flushSync();
    expect(["Send", "Copy Link", "Revoke"].map((l) => t.button(l).disabled)).toEqual([true, true, true]);
    fail(new Error("no phone"));
    await new Promise((r) => setTimeout(r));
    flushSync();
    expect(shown).toEqual([expect.objectContaining({ message: "no phone" })]);
    expect(["Send", "Copy Link", "Revoke"].map((l) => t.button(l).disabled)).toEqual([false, false, false]);
    t.done();
  });

  it("has no extra buttons without actions", () => {
    const t = setup({ title: "Payment Link", fields, primaryLabel: "Copy Link" });
    expect(t.labels()).toEqual(["Cancel", "Copy Link"]);
    t.done();
  });
});
