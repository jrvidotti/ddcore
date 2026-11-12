import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args: any[] = []) => s.replace(/\{(\d+)\}/g, (_, i) => String(args[Number(i)])),
  boot: { data: { lang: "pt-BR", doctypes: {}, site: { currency: "BRL", timezone: "UTC" } } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn(), confirm: vi.fn(), dialog: vi.fn() }));

import Control from "./Control.svelte";

const field = { fieldname: "end_date", fieldtype: "Date", label: "End Date", minDateField: "start_date" };

function setup(initial: string | null, doc: any) {
  const changes: any[] = [];
  const props = $state({ field, value: initial as any, onchange: (v: any) => { changes.push(v); props.value = v; }, doc });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(Control, { target, props });
  flushSync();
  const input = target.querySelector<HTMLInputElement>("input")!;
  const type = (s: string) => {
    input.dispatchEvent(new FocusEvent("focus"));
    input.value = s;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new FocusEvent("blur"));
    flushSync();
  };
  const error = () => target.querySelector(".err")?.textContent ?? "";
  return { props, changes, input, type, error, done: () => { unmount(view); target.remove(); } };
}

describe("a Date with minDateField", () => {
  it("refuses a day before the other field's, and says why", () => {
    const t = setup(null, { start_date: "2026-05-10" });
    t.type("09052026");
    expect(t.changes).toEqual([]);
    expect(t.error()).toBe("Must be on or after 10/05/2026");
    t.type("10052026");
    expect(t.changes.at(-1)).toBe("2026-05-10");
    expect(t.error()).toBe("");
    t.done();
  });

  it("flags a value the other field moved past, without clearing it", () => {
    const t = setup("2026-05-12", { start_date: "2026-05-10" });
    expect(t.error()).toBe("");
    t.props.doc = { start_date: "2026-05-20" };
    flushSync();
    expect(t.error()).toBe("Must be on or after 20/05/2026");
    expect(t.changes).toEqual([]);
    t.done();
  });

  it("takes any day while the other field is empty", () => {
    const t = setup(null, { start_date: null });
    t.type("01012020");
    expect(t.changes.at(-1)).toBe("2020-01-01");
    expect(t.error()).toBe("");
    t.done();
  });
});
