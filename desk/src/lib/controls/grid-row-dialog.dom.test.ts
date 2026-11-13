import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import type { Meta } from "$lib/meta";

vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: null, ready: false } }));
vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "" }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

import { FormController, registerForm } from "$lib/form.svelte";
import Grid from "./Grid.svelte";
import Dialogs from "$lib/components/Dialogs.svelte";

// #162: in a row dialog the child fields' handlers run as the reader types, on the dialog's copy
const childMeta = {
  name: "Campaign Goal",
  fields: [
    { fieldname: "baseline", fieldtype: "Float", label: "Baseline", inListView: true },
    { fieldname: "growth", fieldtype: "Percent", label: "Growth", inListView: true },
    { fieldname: "target", fieldtype: "Float", label: "Target", inListView: true },
  ],
};
const meta = {
  doctype: {
    name: "Campaign", label: "Campaign", formApps: [],
    fields: [{ fieldname: "goals", fieldtype: "Table", label: "Goals", options: "Campaign Goal", gridEditMode: "dialog" }],
  },
  children: { "Campaign Goal": childMeta },
  permissions: { read: true, write: true },
} as unknown as Meta;

const tableChanges: string[][] = [];
registerForm("Campaign", {
  onChange: { goals: (_f, _cdt, _cdn, _row, changed) => { tableChanges.push(changed ?? []); } },
  grids: { goals: { onChange: { growth: (f, row) => { f.setRowValue("goals", row, { target: row.baseline * (1 + row.growth / 100) }); } } } },
});

function render() {
  const frm = new FormController(meta, { doctype: "Campaign", id: "C-1", goals: [{ id: "g1", idx: 1, baseline: 100, growth: 0, target: 100 }] });
  const target = document.createElement("div");
  document.body.appendChild(target);
  const grid = mount(Grid, { target, props: { get frm() { return frm; }, get field() { return frm.field("goals")!; }, childMeta: childMeta as any } });
  const dialogs = mount(Dialogs, { target });
  flushSync();
  return { frm, target, done: () => { unmount(dialogs); unmount(grid); target.remove(); } };
}

function openRow(target: HTMLElement) {
  target.querySelector<HTMLButtonElement>("button[aria-label='Edit row']")!.click();
  flushSync();
  const inputs = [...target.querySelectorAll<HTMLInputElement>("input.num")];
  // the dialog's inputs come after the grid (which shows none: it is dialog only)
  return { baseline: inputs[0], growth: inputs[1], target: inputs[2] };
}

function type(input: HTMLInputElement, value: string) {
  input.dispatchEvent(new FocusEvent("focus"));
  input.value = value;
  input.dispatchEvent(new Event("input", { bubbles: true }));
  input.dispatchEvent(new FocusEvent("blur"));
  flushSync();
}

function button(target: HTMLElement, label: string) {
  return [...target.querySelectorAll<HTMLButtonElement>("button")].find((b) => b.textContent?.trim() === label)!;
}

describe("Grid row dialog", () => {
  it("updates the dialog's other field while the reader types, and the row only on Apply", () => {
    tableChanges.length = 0;
    const { frm, target, done } = render();
    const inputs = openRow(target);
    type(inputs.growth, "10");
    expect(inputs.target.value).toMatch(/^110/);
    expect(frm.doc.goals[0].target).toBe(100);
    expect(tableChanges).toEqual([]);
    button(target, "Apply").click();
    flushSync();
    expect(frm.doc.goals[0]).toMatchObject({ growth: 10 });
    expect(frm.doc.goals[0].target).toBeCloseTo(110);
    expect(tableChanges).toEqual([["growth", "target"]]);
    done();
  });

  it("leaves the row as it was on Cancel", () => {
    tableChanges.length = 0;
    const { frm, target, done } = render();
    const inputs = openRow(target);
    type(inputs.growth, "10");
    button(target, "Cancel").click();
    flushSync();
    expect(frm.doc.goals[0]).toMatchObject({ growth: 0, target: 100 });
    expect(frm.isDirty).toBe(false);
    expect(tableChanges).toEqual([]);
    done();
  });
});
