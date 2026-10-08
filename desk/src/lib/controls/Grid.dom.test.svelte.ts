import { flushSync, mount, tick, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import Grid from "./Grid.svelte";
import { inlineEditable } from "./grid-state";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({
  __: (s: string, args?: any[]) => (args || []).reduce((out: string, a: any, i: number) => out.replace(`{${i}}`, a), s),
  boot: { data: { doctypes: {} } },
}));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
const confirmMock = vi.fn(async () => true);
const dialogs: any[] = [];
const shown: any[] = [];
vi.mock("$lib/ui.svelte", () => ({
  showError: (e: any) => shown.push(e),
  confirm: (...a: any[]) => (confirmMock as any)(...a),
  dialog: (opts: any) => { dialogs.push(opts); return { show: vi.fn(), hide: vi.fn() }; },
}));
const downloads: any[] = [];
vi.mock("$lib/grid-rows", async (orig) => ({ ...(await orig<any>()), downloadTable: (...a: any[]) => downloads.push(a) }));

const childMeta: any = {
  name: "Course Student",
  fields: [
    { fieldname: "employee_name", fieldtype: "Data", label: "Name", inListView: true, readOnly: true },
    { fieldname: "grade", fieldtype: "Float", label: "Grade", inListView: true, readOnly: true },
    { fieldname: "in_class", fieldtype: "Check", label: "In class", inListView: true, readOnly: true },
    { fieldname: "unit", fieldtype: "Data", label: "Unit", hidden: true },
  ],
};

function setup(field: any, perms: Record<string, boolean> = { export: true }, onCellClick: Record<string, (row: any) => void> = {}, editable = true, fieldButtons: Record<string, any[]> = {}, gridActions: Record<string, any[]> = {}) {
  const doc = $state<any>({
    id: "C-1",
    students: [
      { id: "r1", idx: 1, employee_name: "Zoe", grade: 7, in_class: 1, unit: "São Paulo" },
      { id: "r2", idx: 2, employee_name: "Ana", grade: 9, in_class: 0, unit: "Santos" },
      { id: "r3", idx: 3, employee_name: "Bia", grade: 8, in_class: 1, unit: "Sao Carlos" },
    ],
  });
  const selections = new Map<string, { rows: () => any[]; clear: () => void }>();
  const frm: any = {
    doc, doctype: "Course", meta: { permissions: perms }, fieldButtons, gridActions,
    registerGridSelection(fieldname: string, sel: any) { selections.set(fieldname, sel); return () => selections.delete(fieldname); },
    getSelectedRows: (fieldname: string) => selections.get(fieldname)?.rows() ?? [],
    clearSelection: (fieldname: string) => selections.get(fieldname)?.clear(),
    isFieldEditable: () => editable,
    isSetOnce: () => false,
    isFieldMandatory: () => false,
    trigger: vi.fn(),
    cellClickHandlers: (table: string, column: string) => (table === "students" && onCellClick[column] ? [onCellClick[column]] : []),
    clickCell: (_table: string, column: string, row: any) => onCellClick[column](row),
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
  return { doc, frm, target, names, click, done: () => { unmount(view); target.remove(); expect(selections.size).toBe(0); } };
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

  it("hideLabel keeps the label for screen readers only, and still names the row dialog", () => {
    const shown = setup({});
    expect(shown.target.querySelector(".grid-field > .label")?.classList.contains("sr-only")).toBe(false);
    shown.done();
    const t = setup({ hideLabel: true, gridEditMode: "dialog" });
    const label = t.target.querySelector(".grid-field > .label")!;
    expect(label.classList.contains("sr-only")).toBe(true);
    expect(label.textContent).toBe("Students");
    t.click(t.target.querySelector("tbody tr button.icon"));
    expect(dialogs.at(-1).title).toContain("Students");
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

  it("the search box narrows the rows on screen, a hidden field included, AND with the preset filters", () => {
    downloads.length = 0;
    const t = setup({
      gridSelect: true, gridExport: true, gridSort: { field: "employee_name" }, gridSearch: ["employee_name", "unit"],
      gridFilters: [{ label: "In class", filters: { in_class: 1 } }],
    });
    const box = t.target.querySelector<HTMLInputElement>(".grid-toolbar input.grid-search")!;
    const type = (q: string) => { box.value = q; box.dispatchEvent(new Event("input", { bubbles: true })); flushSync(); };
    type("sao");
    expect(t.names()).toEqual(["Bia", "Zoe"]);
    type("SÃO zo");
    expect(t.names()).toEqual(["Zoe"]);
    type("s");
    expect(t.names()).toEqual(["Ana", "Bia", "Zoe"]);
    t.click(t.target.querySelector("button.grid-filter"));
    type("santos");
    expect(t.target.querySelector("tbody")?.textContent).toContain("No rows match the filters");
    type("");
    t.click(t.target.querySelector("thead input[type=checkbox]"));
    expect(t.target.querySelector(".grid-toolbar")?.textContent).toContain("2 selected");
    type("bia");
    t.click([...t.target.querySelectorAll(".grid-toolbar button")].find((b) => b.textContent?.includes("CSV"))!);
    // Zoe stays selected but off screen: select all and export follow what is shown
    expect(downloads[0][3].map((r: any[]) => r[0])).toEqual(["Bia"]);
    expect(t.doc.students.map((r: any) => r.idx)).toEqual([1, 2, 3]);
    t.done();
  });

  it("no gridSearch, no search box", () => {
    const t = setup({ gridExport: true });
    expect(t.target.querySelector("input.grid-search")).toBeNull();
    t.done();
  });

  it("a cell with an onCellClick handler is a button that hands over the row clicked, not the one at its position", () => {
    const clicked: any[] = [];
    const t = setup({ gridSort: { field: "employee_name" }, gridFilters: [{ label: "In class", filters: { in_class: 1 }, default: true }] }, {}, { employee_name: (row) => clicked.push(row) });
    const buttons = [...t.target.querySelectorAll<HTMLButtonElement>("tbody button.cell-click")];
    expect(buttons.map((b) => b.textContent?.trim())).toEqual(["Bia", "Zoe"]);
    t.click(buttons[1]);
    expect(clicked).toEqual([t.doc.students[0]]);
    // the other columns have no handler and stay text
    expect(t.target.querySelectorAll("tbody td button.cell-click")).toHaveLength(2);
    t.done();
  });

  it("an editable cell keeps its control, even with a handler", () => {
    childMeta.fields[0].readOnly = false;
    try {
      const t = setup({}, {}, { employee_name: () => {} });
      expect(t.target.querySelectorAll("tbody button.cell-click")).toHaveLength(0);
      expect(t.target.querySelectorAll("tbody input")).not.toHaveLength(0);
      t.done();
    } finally {
      childMeta.fields[0].readOnly = true;
    }
  });

  it("an edit in a cell tells onChange the row and the field it changed", () => {
    childMeta.fields[0].readOnly = false;
    try {
      const t = setup({});
      const input = t.target.querySelector<HTMLInputElement>("tbody tr:nth-child(2) input")!;
      input.value = "Ana Maria";
      input.dispatchEvent(new Event("input", { bubbles: true }));
      input.dispatchEvent(new Event("change", { bubbles: true }));
      flushSync();
      expect(t.doc.students[1].employee_name).toBe("Ana Maria");
      expect(t.frm.trigger).toHaveBeenCalledWith("students", "Course Student", "r2", t.doc.students[1], ["employee_name"]);
      t.done();
    } finally {
      childMeta.fields[0].readOnly = true;
    }
  });

  it("the row dialog tells onChange which row and fields it changed", () => {
    dialogs.length = 0;
    const t = setup({ gridEditMode: "dialog" });
    t.click(t.target.querySelector("tbody tr:nth-child(2) button.icon"));
    const d = dialogs.at(-1);
    d.primaryAction({ ...t.doc.students[1], grade: 10 }, { hide: vi.fn() });
    expect(t.doc.students[1].grade).toBe(10);
    expect(t.frm.trigger).toHaveBeenCalledWith("students", "Course Student", "r2", t.doc.students[1], ["grade"]);
    t.done();
  });

  it("a dialog-mode cell with a handler is a button too", () => {
    const clicked: any[] = [];
    const t = setup({ gridEditMode: "dialog" }, {}, { grade: (row) => clicked.push(row.employee_name) }, false);
    t.click(t.target.querySelector("tbody tr:nth-child(3) button.cell-click"));
    expect(clicked).toEqual(["Bia"]);
    t.done();
  });

  it("a Signature column is never edited in its cell: it says Signed and opens the row dialog", () => {
    expect(inlineEditable("Signature")).toBe(false);
    expect(inlineEditable("Geolocation")).toBe(false);
    expect(inlineEditable("Data")).toBe(true);
    expect(inlineEditable(undefined)).toBe(true);
    childMeta.fields.push({ fieldname: "signed", fieldtype: "Signature", label: "Signed", inListView: true });
    try {
      dialogs.length = 0;
      const t = setup({});
      t.doc.students[1].signed = "data:image/png;base64,AAAA";
      flushSync();
      const cell = t.target.querySelector("tbody tr:nth-child(2) td:last-of-type")!.previousElementSibling!;
      expect(cell.querySelector("canvas, input")).toBe(null);
      expect(cell.textContent?.trim()).toBe("Signed");
      expect(t.target.querySelector("tbody tr:nth-child(1) td:last-of-type")!.previousElementSibling!.textContent?.trim()).toBe("—");
      t.click(cell.querySelector("button.cell-click"));
      expect(dialogs).toHaveLength(1);
      expect(dialogs[0].values.id).toBe("r2");
      t.done();
    } finally {
      childMeta.fields.pop();
    }
  });
});

describe("Grid field buttons", () => {
  it("shows a button added to the Table field in the grid's toolbar, which it opens by itself", () => {
    const onClick = vi.fn();
    const g = setup({}, {}, {}, true, { students: [{ label: "New student", onClick }] });
    const button = g.target.querySelector(".grid-toolbar .field-btn");
    expect(button?.textContent?.trim()).toBe("New student");
    g.click(button);
    expect(onClick).toHaveBeenCalledOnce();
    g.done();
  });

  it("has no toolbar when the field has neither a button nor a grid feature", () => {
    const g = setup({}, {});
    expect(g.target.querySelector(".grid-toolbar")).toBeNull();
    g.done();
  });
});

describe("Grid actions on the selected rows", () => {
  const boxes = (t: any) => [...t.target.querySelectorAll("tbody input[type=checkbox]")] as HTMLInputElement[];
  const actionButtons = (t: any) => [...t.target.querySelectorAll(".grid-toolbar .grid-action")] as HTMLButtonElement[];
  const search = (t: any, q: string) => {
    const box = t.target.querySelector(".grid-toolbar input.grid-search") as HTMLInputElement;
    box.value = q; box.dispatchEvent(new Event("input", { bubbles: true })); flushSync();
  };

  it("frm.getSelectedRows gives the selected rows still on screen, in screen order, and clearSelection empties them", () => {
    const t = setup({ gridSelect: true, gridSort: { field: "grade", order: "desc" }, gridSearch: ["employee_name"] });
    expect(t.frm.getSelectedRows("students")).toEqual([]);
    t.click(t.target.querySelector("thead input[type=checkbox]"));
    expect(t.frm.getSelectedRows("students").map((r: any) => r.employee_name)).toEqual(["Ana", "Bia", "Zoe"]);
    search(t, "a"); // Zoe leaves the screen but stays ticked
    expect(t.frm.getSelectedRows("students").map((r: any) => r.employee_name)).toEqual(["Ana", "Bia"]);
    expect(t.frm.getSelectedRows("students")[0]).toBe(t.doc.students[1]);
    t.frm.clearSelection("students");
    flushSync();
    expect(t.frm.getSelectedRows("students")).toEqual([]);
    expect(boxes(t).some((b) => b.checked)).toBe(false);
    t.done();
  });

  it("shows each action with the count of selected rows it applies to, only while some do", () => {
    const enable = { label: "Enable", primary: true, condition: (r: any) => !r.in_class, onClick: vi.fn() };
    const roles = { label: "Set Roles", onClick: vi.fn() };
    const t = setup({ gridSelect: true }, {}, {}, false, {}, { students: [enable, roles] });
    expect(actionButtons(t)).toEqual([]);
    t.click(boxes(t)[0]); // Zoe, in class
    expect(actionButtons(t).map((b) => b.textContent?.trim())).toEqual(["Set Roles (1)"]);
    t.click(boxes(t)[1]); // Ana
    expect(actionButtons(t).map((b) => [b.textContent?.trim(), b.classList.contains("primary")])).toEqual([["Enable (1)", true], ["Set Roles (2)", false]]);
    t.done();
  });

  it("hands the action its rows, keeps the buttons disabled while it runs, then clears the selection", async () => {
    let finish!: () => void;
    const onClick = vi.fn(() => new Promise<void>((r) => (finish = r)));
    const t = setup({ gridSelect: true, gridSort: { field: "employee_name" } }, {}, {}, true, {}, { students: [{ label: "Enable", onClick }, { label: "Disable", onClick: vi.fn() }] });
    t.click(t.target.querySelector("thead input[type=checkbox]"));
    t.click(actionButtons(t)[0]);
    expect(onClick).toHaveBeenCalledWith([t.doc.students[1], t.doc.students[2], t.doc.students[0]]);
    expect(actionButtons(t).every((b) => b.disabled)).toBe(true);
    finish();
    await tick(); await tick();
    flushSync();
    expect(actionButtons(t)).toEqual([]);
    expect(t.frm.getSelectedRows("students")).toEqual([]);
    expect(t.target.querySelector(".grid-toolbar")).toBeNull();
    t.done();
  });

  it("shows a rejection and still clears the selection", async () => {
    shown.length = 0;
    const err = new Error("not allowed");
    const t = setup({ gridSelect: true }, {}, {}, true, {}, { students: [{ label: "Enable", onClick: async () => { throw err; } }] });
    t.click(boxes(t)[1]);
    t.click(actionButtons(t)[0]);
    await tick(); await tick();
    flushSync();
    expect(shown).toEqual([err]);
    expect(t.frm.getSelectedRows("students")).toEqual([]);
    t.done();
  });

  it("no gridSelect, no actions", () => {
    const t = setup({}, {}, {}, true, {}, { students: [{ label: "Enable", onClick: vi.fn() }] });
    expect(t.target.querySelector("input[type=checkbox]")).toBeNull();
    expect(actionButtons(t)).toEqual([]);
    t.done();
  });
});
