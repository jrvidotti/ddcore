import { flushSync, mount, unmount } from "svelte";
import { afterEach, describe, expect, it, vi } from "vitest";

const shown: any[] = [];
vi.mock("$lib/ui.svelte", () => ({ showError: (e: any) => shown.push(e) }));

import FormBanner from "./FormBanner.svelte";
import { FormController } from "$lib/form.svelte";

let view: ReturnType<typeof mount> | undefined;
const open = (banner: any) => { view = mount(FormBanner, { target: document.body, props: { banner } }); flushSync(); };
afterEach(() => { if (view) unmount(view); view = undefined; document.body.innerHTML = ""; shown.length = 0; });

describe("FormBanner", () => {
  it("shows the message in its colour, blue when none is given", () => {
    open({ message: "Totals differ", color: "orange" });
    const el = document.querySelector(".form-banner")!;
    expect(el.textContent).toContain("Totals differ");
    expect(el.classList.contains("orange")).toBe(true);
    unmount(view!); view = undefined; document.body.innerHTML = "";
    open({ message: "Note" });
    expect(document.querySelector(".form-banner")!.classList.contains("blue")).toBe(true);
  });

  it("runs an action, with every button disabled until it settles", async () => {
    let finish!: () => void;
    const onClick = vi.fn(() => new Promise<void>((r) => { finish = r; }));
    open({ message: "m", actions: [{ label: "Fix", onClick, primary: true }, { label: "Other", onClick: vi.fn() }] });
    const buttons = [...document.querySelectorAll<HTMLButtonElement>(".form-banner button")];
    expect(buttons.map((b) => b.textContent)).toEqual(["Fix", "Other"]);
    expect(buttons[0].classList.contains("primary")).toBe(true);
    buttons[0].click(); flushSync();
    expect(onClick).toHaveBeenCalledOnce();
    expect(buttons.every((b) => b.disabled)).toBe(true);
    finish(); await Promise.resolve(); await Promise.resolve(); flushSync();
    expect(buttons.every((b) => !b.disabled)).toBe(true);
  });

  it("shows what an action throws", async () => {
    open({ message: "m", actions: [{ label: "Fix", onClick: () => { throw new Error("no"); } }] });
    document.querySelector<HTMLButtonElement>(".form-banner button")!.click();
    await Promise.resolve(); flushSync();
    expect(shown).toHaveLength(1);
  });
});

describe("frm.setBanner and frm.setGridDfProperty", () => {
  const frm = () => new FormController({ doctype: { name: "Course", fields: [] }, permissions: {} } as any, { id: "C-1" });

  it("setBanner keeps the notice until null or the next refresh clears it", () => {
    const f = frm();
    f.setBanner("Totals differ", { color: "orange" });
    expect(f.banner).toEqual({ message: "Totals differ", color: "orange" });
    f.setBanner(null);
    expect(f.banner).toBeNull();
    f.setBanner("again");
    f.clearButtons();
    expect(f.banner).toBeNull();
  });

  it("setGridDfProperty gathers the overrides by table and child field", () => {
    const f = frm();
    f.setGridDfProperty("students", "grade", "hidden", true);
    f.setGridDfProperty("students", "grade", "label", "Mark");
    f.setGridDfProperty("students", "unit", "label", "Campus");
    expect(f.gridDfProps).toEqual({ students: { grade: { hidden: true, label: "Mark" }, unit: { label: "Campus" } } });
  });
});
