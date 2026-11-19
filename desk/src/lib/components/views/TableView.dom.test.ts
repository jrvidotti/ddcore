import { flushSync, mount, unmount } from "svelte";
import { beforeEach, describe, expect, it, vi } from "vitest";
import TableView from "./TableView.svelte";
import type { ListViewOptions } from "$lib/desk-sdk";
import type { Field, Meta } from "$lib/meta";

const goto = vi.fn();
vi.mock("$app/navigation", () => ({ goto: (url: string) => goto(url) }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, doctypeLabel: (s: string) => s }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "" }));

const fields: Field[] = [
  { fieldname: "job_title_name", fieldtype: "Data", label: "Name" },
  { fieldname: "active_employees", fieldtype: "Int", label: "Active Employees" },
] as Field[];

const meta = {
  doctype: { name: "Job Title", app: "hr", label: "Job Title", idGeneration: { hash: true }, titleField: "job_title_name", fields },
  permissions: {},
} as unknown as Meta;

function render(rows: any[], settings: ListViewOptions) {
  const target = document.createElement("div");
  const view = mount(TableView, {
    target,
    props: {
      rows, meta, doctype: "Job Title", wsPrefix: "/app/HR", columns: fields, showID: false, showModified: false,
      selected: new Set<string>(), orderBy: "", settings, onSort: () => {}, onToggle: () => {}, onSelectAll: () => {},
      cellText: (row: any, f: Field) => String(row[f.fieldname!] ?? ""),
    },
  });
  flushSync();
  return { target, view };
}

const countCell = (target: HTMLElement, row = 0) => target.querySelectorAll("tbody tr")[row].querySelectorAll("td")[2];

describe("TableView cellLinks", () => {
  beforeEach(() => goto.mockClear());

  it("links the cell to the URL the function returns, without opening the row", () => {
    const { target, view } = render([{ id: "jt1", job_title_name: "Seller", active_employees: 12 }], {
      cellLinks: { active_employees: (_v, row) => `/app/HR/Employee?job_title=${row.id}&status=Active` },
    });
    const a = countCell(target).querySelector("a")!;
    expect(a.getAttribute("href")).toBe("/app/HR/Employee?job_title=jt1&status=Active");
    expect(a.textContent).toBe("12");
    a.addEventListener("click", (e) => e.preventDefault()); // no real navigation in jsdom
    a.click();
    expect(goto).not.toHaveBeenCalled();
    unmount(view);
  });

  it("leaves the cell as text when the function returns nothing or throws", () => {
    const { target, view } = render([
      { id: "jt1", job_title_name: "Seller", active_employees: 0 },
      { id: "jt2", job_title_name: "Cashier", active_employees: 3 },
    ], {
      cellLinks: { active_employees: (v) => { if (v === 3) throw new Error("boom"); return v ? "/x" : null; } },
    });
    for (const row of [0, 1]) {
      expect(countCell(target, row).querySelector("a")).toBeNull();
    }
    expect(countCell(target, 1).textContent?.trim()).toBe("3");
    countCell(target).click();
    expect(goto).toHaveBeenCalledWith("/app/HR/JobTitle/jt1");
    unmount(view);
  });
});
