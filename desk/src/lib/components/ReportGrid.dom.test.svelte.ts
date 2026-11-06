import { flushSync, mount, tick, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import ReportGrid from "./ReportGrid.svelte";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: { doctypes: {} } } }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));
const shown: any[] = [];
vi.mock("$lib/ui.svelte", () => ({ showError: (e: any) => shown.push(e) }));

function setup(props: Record<string, any> = {}) {
  const target = document.createElement("div");
  document.body.append(target);
  const view: any = mount(ReportGrid, {
    target,
    props: {
      columns: [{ fieldname: "title", fieldtype: "Data", label: "Title" }],
      rows: [{ title: "Morning class" }],
      wsPrefix: "/app", filename: "classes", ...props,
    },
  });
  flushSync();
  const click = (el: Element | null) => { (el as HTMLElement).click(); flushSync(); };
  return { target, view, click, done: () => { unmount(view); target.remove(); } };
}

describe("ReportGrid field buttons", () => {
  it("shows a Report field's buttons in the toolbar, which they open by themselves", () => {
    const onClick = vi.fn();
    const g = setup({ buttons: [{ label: "New Class", onClick }] });
    const button = g.target.querySelector<HTMLElement>(".grid-toolbar .field-btn");
    expect(button?.textContent?.trim()).toBe("New Class");
    button!.click();
    expect(onClick).toHaveBeenCalledOnce();
    g.done();
  });

  it("has no toolbar without buttons or a grid feature", () => {
    const g = setup();
    expect(g.target.querySelector(".grid-toolbar")).toBeNull();
    g.done();
  });
});

describe("ReportGrid cell clicks", () => {
  const columns = [
    { fieldname: "id", fieldtype: "Data", label: "ID" },
    { fieldname: "action", fieldtype: "Data", label: "Action" },
  ];
  const rows = [{ id: "B", action: "Refund" }, { id: "A", action: "" }, { id: "C", action: "Send Back" }];

  it("makes a clickable column's non-empty cells buttons that hand over the row clicked", () => {
    const oncellclick = vi.fn();
    const g = setup({ columns, rows, clickable: new Set(["action"]), oncellclick, baseSort: { field: "id", order: "asc" } });
    const buttons = g.target.querySelectorAll<HTMLElement>("tbody button.cell-click");
    expect([...buttons].map((b) => b.textContent?.trim())).toEqual(["Refund", "Send Back"]);
    buttons[1].click();
    expect(oncellclick).toHaveBeenCalledWith("action", rows[2]);
    g.done();
  });

  it("takes a click anywhere in a clickable cell, and none in an empty one", () => {
    const oncellclick = vi.fn();
    const onrowclick = vi.fn();
    const g = setup({ columns, rows, clickable: new Set(["action"]), oncellclick, onrowclick });
    const cells = [...g.target.querySelectorAll<HTMLElement>("tbody tr")].map((tr) => tr.querySelectorAll("td")[1]);
    cells[0].click();
    expect(oncellclick).toHaveBeenCalledWith("action", rows[0]);
    expect(onrowclick).not.toHaveBeenCalled();
    cells[1].click(); // "A" has no action: its empty cell is the row's
    expect(oncellclick).toHaveBeenCalledTimes(1);
    expect(onrowclick).toHaveBeenCalledWith(rows[1]);
    g.done();
  });

  it("hands a row click the row, but not a click on a link or a cell button", () => {
    const linkColumns = [{ fieldname: "customer", fieldtype: "Link", options: "Customer", label: "Customer" }, ...columns];
    const linkRows = [{ customer: "CUST-1", id: "B", action: "Refund" }];
    const onrowclick = vi.fn();
    const g = setup({ columns: linkColumns, rows: linkRows, clickable: new Set(["action"]), oncellclick: vi.fn(), onrowclick });
    const tr = g.target.querySelector<HTMLElement>("tbody tr")!;
    expect(tr.classList.contains("row-click")).toBe(true);
    tr.querySelector("a")!.addEventListener("click", (e) => e.preventDefault());
    tr.querySelector<HTMLElement>("a")!.click();
    tr.querySelector<HTMLElement>("button.cell-click")!.click();
    expect(onrowclick).not.toHaveBeenCalled();
    tr.querySelectorAll<HTMLElement>("td")[1].click();
    expect(onrowclick).toHaveBeenCalledWith(linkRows[0]);
    g.done();
  });

  it("has no clickable rows without onrowclick", () => {
    const g = setup({ columns, rows });
    expect(g.target.querySelector("tr.row-click")).toBeNull();
    g.done();
  });

  it("has no cell buttons without clickable columns", () => {
    const g = setup({ columns, rows });
    expect(g.target.querySelector("tbody button")).toBeNull();
    g.done();
  });
});

describe("ReportGrid selection and actions", () => {
  const columns = [{ fieldname: "id", fieldtype: "Data", label: "ID" }, { fieldname: "paid", fieldtype: "Check", label: "Paid" }];
  const rows = [{ id: "B", paid: 0 }, { id: "A", paid: 1 }, { id: "C", paid: 0 }];
  const actionButtons = (g: any) => [...g.target.querySelectorAll(".grid-toolbar .grid-action")] as HTMLButtonElement[];

  it("hands out the selected rows on screen, in screen order, and clears them", () => {
    const g = setup({ columns, rows, selectable: true, baseSort: { field: "id", order: "asc" } });
    expect(g.view.selectedRows()).toEqual([]);
    g.click(g.target.querySelector("thead input[type=checkbox]"));
    expect(g.view.selectedRows()).toEqual([rows[1], rows[0], rows[2]]);
    g.view.clearSelection();
    flushSync();
    expect(g.view.selectedRows()).toEqual([]);
    expect(g.target.querySelector(".grid-toolbar")).toBeNull();
    g.done();
  });

  it("runs an action on the selected rows it applies to, then clears the selection", async () => {
    const onClick = vi.fn(async () => {});
    const g = setup({ columns, rows, selectable: true, actions: [{ label: "Charge", condition: (r: any) => !r.paid, onClick }] });
    g.click(g.target.querySelector("thead input[type=checkbox]"));
    expect(actionButtons(g).map((b) => b.textContent?.trim())).toEqual(["Charge (2)"]);
    g.click(actionButtons(g)[0]);
    expect(onClick).toHaveBeenCalledWith([rows[0], rows[2]]);
    await tick(); await tick();
    flushSync();
    expect(g.view.selectedRows()).toEqual([]);
    expect(shown).toEqual([]);
    g.done();
  });
});

describe("ReportGrid footer", () => {
  const columns = [
    { fieldname: "group", fieldtype: "Data", label: "Group" },
    { fieldname: "customers", fieldtype: "Int", label: "Customers" },
    { fieldname: "delta", fieldtype: "Float", label: "Δ year (pp)" },
  ];
  const rows = [{ group: "Band 1", customers: 10, delta: 1.5 }, { group: "Band 2", customers: 20, delta: -0.5 }];
  const footer = (target: Element) => [...target.querySelectorAll("tfoot td")].map((td) => td.textContent?.trim());

  it("sums every numeric column when the report sends no totals", () => {
    const g = setup({ columns, rows });
    expect(footer(g.target)).toEqual(["Total", "30", "1"]);
    g.done();
  });

  it("draws the report's own totals row, and leaves the columns it omits empty", () => {
    const g = setup({ columns, rows, reportTotals: { customers: 25 } });
    expect(footer(g.target)).toEqual(["Total", "25", ""]);
    g.done();
  });

  it("draws the report's label in the first column, and no footer for `totals: {}`", () => {
    const g = setup({ columns, rows, reportTotals: { group: "All customers", customers: 25 } });
    expect(footer(g.target)[0]).toBe("All customers");
    g.done();
    const none = setup({ columns, rows, reportTotals: {} });
    expect(none.target.querySelector("tfoot")).toBeNull();
    none.done();
  });

  it("leaves out the report's totals while the search hides rows", async () => {
    const g = setup({ columns, rows, reportTotals: { customers: 25 }, search: ["group"] });
    const input = g.target.querySelector<HTMLInputElement>(".grid-toolbar input")!;
    input.value = "Band 1";
    input.dispatchEvent(new Event("input", { bubbles: true }));
    await tick();
    flushSync();
    expect(g.target.querySelectorAll("tbody tr")).toHaveLength(1);
    expect(g.target.querySelector("tfoot")).toBeNull();
    g.done();
  });
});

describe("ReportGrid row drill-down", () => {
  it("makes clickable only the rows rowclickable accepts", () => {
    const rows = [{ title: "Band 6", drill: { band: "6" } }, { title: "No band" }];
    const onrowclick = vi.fn();
    const g = setup({ rows, onrowclick, rowclickable: (r: any) => !!r.drill });
    const trs = [...g.target.querySelectorAll<HTMLElement>("tbody tr")];
    expect(trs.map((tr) => tr.classList.contains("row-click"))).toEqual([true, false]);
    g.click(trs[1].querySelector("td"));
    expect(onrowclick).not.toHaveBeenCalled();
    g.click(trs[0].querySelector("td"));
    expect(onrowclick).toHaveBeenCalledWith(rows[0]);
    g.done();
  });
});
