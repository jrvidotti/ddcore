import { flushSync, mount, tick, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import TableMultiSelectControl from "./TableMultiSelectControl.svelte";

const linkSearch = vi.fn(async () => [{ id: "red", title: "Red" }, { id: "blue", title: "Blue" }, { id: "green", title: "Green" }]);
vi.mock("$lib/api", () => ({ api: { linkSearch: (...a: any[]) => (linkSearch as any)(...a) } }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args?: any[]) => (args || []).reduce((out: string, a: any, i: number) => out.replace(`{${i}}`, a), s),
  boot: { data: { doctypes: { Tag: { label: "Tag", titleField: "title" } } } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: (_: string, id: string) => id.toUpperCase(), setLinkTitle: vi.fn() }));
vi.mock("$lib/meta", () => ({ getMeta: vi.fn() }));
vi.mock("$app/state", () => ({ page: { params: {} } }));
vi.mock("$lib/components/sidebar-workspace", () => ({ getRememberedWorkspace: () => "" }));

// jsdom has no ResizeObserver, which the dropdown's positioning watches
globalThis.ResizeObserver ||= class { observe() {} unobserve() {} disconnect() {} } as any;

const childMeta: any = { name: "Note Tag", fields: [{ fieldname: "tag", fieldtype: "Link", options: "Tag" }] };

function setup(initial: any[], readOnly = false) {
  const props = $state({
    field: { fieldname: "tags", fieldtype: "Table MultiSelect", options: "Note Tag" } as any,
    value: initial as any, readOnly, childMeta, onchange: (v: any) => (props.value = v),
  });
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(TableMultiSelectControl, { target, props });
  flushSync();
  const pills = () => [...target.querySelectorAll(".ms-pill")].map((p) => p.textContent!.replace("×", "").trim());
  return { props, target, pills, done: () => { unmount(view); target.remove(); } };
}

describe("TableMultiSelectControl", () => {
  it("shows each value as a pill with its title, and removes one", () => {
    const t = setup([{ id: "r1", tag: "red", idx: 1 }, { id: "r2", tag: "blue", idx: 2 }]);
    expect(t.pills()).toEqual(["RED", "BLUE"]);
    (t.target.querySelector(".ms-remove") as HTMLButtonElement).click();
    flushSync();
    expect(t.props.value).toEqual([{ id: "r2", tag: "blue", idx: 1 }]);
    expect(t.pills()).toEqual(["BLUE"]);
    t.done();
  });

  it("offers only what is not chosen yet, and adds a row for a pick", async () => {
    const t = setup([{ id: "r1", tag: "red", idx: 1 }]);
    const input = t.target.querySelector("input")!;
    input.dispatchEvent(new FocusEvent("focus"));
    await vi.waitFor(() => expect(t.target.querySelectorAll("[role=option]").length).toBe(2));
    const options = [...t.target.querySelectorAll("[role=option]")];
    expect(options.map((o) => o.textContent)).toEqual(["Blue", "Green"]);
    options[1].dispatchEvent(new MouseEvent("mousedown", { bubbles: true }));
    flushSync();
    expect(t.props.value.map((r: any) => r.tag)).toEqual(["red", "green"]);
    expect(t.props.value[1]).toMatchObject({ doctype: "Note Tag", tag: "green", idx: 2 });
    t.done();
  });

  it("removes the last value with Backspace on an empty input", async () => {
    const t = setup([{ tag: "red" }, { tag: "blue" }]);
    const input = t.target.querySelector("input")!;
    input.dispatchEvent(new KeyboardEvent("keydown", { key: "Backspace", bubbles: true, cancelable: true }));
    flushSync();
    expect(t.pills()).toEqual(["RED"]);
    t.done();
  });

  it("is only pills, linking to the record, when read-only", async () => {
    const t = setup([{ tag: "red" }], true);
    await tick();
    expect(t.target.querySelector("input")).toBeNull();
    expect(t.target.querySelector(".ms-pill a")?.getAttribute("href")).toBe("/app/Tag/red");
    t.done();
  });
});
