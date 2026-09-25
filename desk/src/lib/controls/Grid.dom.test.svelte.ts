import { flushSync, mount, tick, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import Grid from "./Grid.svelte";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args?: any[]) => (args || []).reduce((out: string, a: any, i: number) => out.replace(`{${i}}`, a), s),
  boot: { data: { doctypes: {} } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
const confirmMock = vi.fn(async () => true);
vi.mock("$lib/ui.svelte", () => ({ confirm: (...a: any[]) => (confirmMock as any)(...a), dialog: vi.fn() }));
const downloads: any[] = [];
vi.mock("$lib/grid-rows", async (orig) => ({ ...(await orig<any>()), downloadTable: (...a: any[]) => downloads.push(a) }));

const childMeta: any = {
  name: "Course Student",
  fields: [
    { fieldname: "employee_name", fieldtype: "Data", label: "Name", inListView: true, readOnly: true },
    { fieldname: "grade", fieldtype: "Float", label: "Grade", inListView: true, readOnly: true },
    { fieldname: "in_class", fieldtype: "Check", label: "In class", inListView: true, readOnly: true },
  ],
};

function setup(field: any, perms: Record<string, boolean> = { export: true }) {
  const doc = $state<any>({
    id: "C-1",
    students: [
      { id: "r1", idx: 1, employee_name: "Zoe", grade: 7, in_class: 1 },
      { id: "r2", idx: 2, employee_name: "Ana", grade: 9, in_class: 0 },
      { id: "r3", idx: 3, employee_name: "Bia", grade: 8, in_class: 1 },
    ],
  });
  const frm: any = {
    doc, doctype: "Course", meta: { permissions: perms },
    isFieldEditable: () => true,
    isFieldMandatory: () => false,
    trigger: vi.fn(),
    removeChild(fieldname: string, i: number) {
      doc[fieldname].splice(i, 1);
      doc[fieldname].forEach((r: any, n: number) => (r.idx = n + 1));
    },
  };
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(Grid, { target, props: { frm, field: { fieldname: "students", fieldtype: "Table", label: "Students", ...field }, childMeta } });
  flushSync();
  const names = () => [...target.querySelectorAll("tbody tr")].map((tr) => tr.querySelectorAll("td")[field.gridSelect ? 2 : 1]?.textContent?.trim());
  const click = (el: Element | null) => { (el as HTMLElement).click(); flushSync(); };
  return { doc, target, names, click, done: () => { unmount(view); target.remove(); } };
}

describe("Grid", () => {
  it("shows the rows in gridSort order without touching idx, and sorts by header", () => {
    const t = setup({ gridSort: { field: "employee_name" }, gridSortable: true });
    expect(t.names()).toEqual(["Ana", "Bia", "Zoe"]);
    expect(t.doc.students.map((r: any) => r.idx)).toEqual([1, 2, 3]);
    const headers = t.target.querySelectorAll("thead button.grid-sort");
    t.click(headers[1]); // grade asc
    expect(t.names()).toEqual(["Zoe", "Bia", "Ana"]);
    t.click(headers[1]); // grade desc
    expect(t.names()).toEqual(["Ana", "Bia", "Zoe"]);
    t.click(headers[1]); // back to the default
    expect(t.names()).toEqual(["Ana", "Bia", "Zoe"]);
    expect(t.target.querySelector("thead th[aria-sort=ascending]")?.textContent).toContain("Name");
    t.done();
  });

  it("shows the # column unless gridIndex is false", () => {
    const shown = setup({});
    expect(shown.target.querySelector("thead")?.textContent).toContain("#");
    shown.done();
    const t = setup({ gridSort: { field: "employee_name" }, gridIndex: false });
    expect(t.target.querySelector("thead")?.textContent).not.toContain("#");
    expect(t.target.querySelector("tbody tr td")?.textContent?.trim()).toBe("Ana");
    t.done();
  });

  it("removes the row clicked, not the one at its position on screen", () => {
    const t = setup({ gridSort: { field: "employee_name" } });
    // first on screen is Ana, which is second in the document
    t.click(t.target.querySelector("tbody tr button.danger"));
    expect(t.doc.students.map((r: any) => r.employee_name)).toEqual(["Zoe", "Bia"]);
    expect(t.names()).toEqual(["Bia", "Zoe"]);
    t.done();
  });

  it("selects rows, deletes the selected ones and exports the rest in screen order", async () => {
    downloads.length = 0;
    const t = setup({ gridSort: { field: "grade", order: "desc" }, gridSelect: true, gridExport: true });
    const boxes = () => [...t.target.querySelectorAll<HTMLInputElement>("tbody input[type=checkbox]")];
    t.click(boxes()[0]); // Ana (9)
    t.click(boxes()[2]); // Zoe (7)
    expect(t.target.querySelector(".grid-toolbar")?.textContent).toContain("2 selected");
    t.click([...t.target.querySelectorAll(".grid-toolbar button")].find((b) => b.textContent?.includes("CSV"))!);
    expect(downloads[0][0]).toBe("csv");
    expect(downloads[0][1]).toBe("Course-C-1-students");
    expect(downloads[0][3]).toEqual([["Ana", 9, false], ["Zoe", 7, true]]);
    t.click([...t.target.querySelectorAll(".grid-toolbar button")].find((b) => b.textContent?.includes("Delete selected"))!);
    await tick(); await tick();
    flushSync();
    expect(confirmMock).toHaveBeenCalled();
    expect(t.doc.students.map((r: any) => [r.employee_name, r.idx])).toEqual([["Bia", 1]]);
    expect(t.target.querySelector(".grid-toolbar")?.textContent).not.toContain("selected");
    t.done();
  });

  it("select all takes every row, and export is hidden without the export permission", () => {
    const t = setup({ gridSelect: true, gridExport: true }, { export: false });
    t.click(t.target.querySelector("thead input[type=checkbox]"));
    expect(t.target.querySelector(".grid-toolbar")?.textContent).toContain("3 selected");
    expect(t.target.textContent).not.toContain("XLSX");
    t.done();
  });

  it("preset filters narrow the rows on screen, and select all and export follow them", () => {
    downloads.length = 0;
    const t = setup({
      gridSelect: true, gridExport: true, gridSort: { field: "employee_name" },
      gridFilters: [
        { label: "In class", filters: [["in_class", "=", 1]], default: true },
        { label: "Good grade", filters: { grade: 9 } },
      ],
    });
    const toggles = () => [...t.target.querySelectorAll<HTMLButtonElement>("button.grid-filter")];
    expect(toggles().map((b) => [b.textContent, b.getAttribute("aria-pressed")])).toEqual([["In class", "true"], ["Good grade", "false"]]);
    expect(t.names()).toEqual(["Bia", "Zoe"]);
    t.click(t.target.querySelector("thead input[type=checkbox]"));
    expect(t.target.querySelector(".grid-toolbar")?.textContent).toContain("2 selected");
    t.click([...t.target.querySelectorAll(".grid-toolbar button")].find((b) => b.textContent?.includes("CSV"))!);
    expect(downloads[0][3].map((r: any[]) => r[0])).toEqual(["Bia", "Zoe"]);
    t.click(toggles()[1]); // AND with "Good grade": nothing left
    expect(t.names()).toEqual([undefined]);
    expect(t.target.querySelector("tbody")?.textContent).toContain("No rows match the filters");
    t.click(toggles()[0]); // only "Good grade"
    expect(t.names()).toEqual(["Ana"]);
    t.click(toggles()[1]);
    expect(t.names()).toEqual(["Ana", "Bia", "Zoe"]);
    expect(t.doc.students.map((r: any) => r.idx)).toEqual([1, 2, 3]);
    t.done();
  });
});
