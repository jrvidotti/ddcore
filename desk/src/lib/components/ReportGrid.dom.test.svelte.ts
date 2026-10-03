import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import ReportGrid from "./ReportGrid.svelte";

vi.mock("$lib/api", () => ({ api: {} }));
vi.mock("$lib/boot.svelte", () => ({ __: (s: string) => s, boot: { data: { doctypes: {} } } }));
vi.mock("$lib/titles.svelte", () => ({ getLinkTitle: () => "", setLinkTitle: vi.fn() }));

function setup(props: Record<string, any> = {}) {
  const target = document.createElement("div");
  document.body.append(target);
  const view = mount(ReportGrid, {
    target,
    props: {
      columns: [{ fieldname: "title", fieldtype: "Data", label: "Title" }],
      rows: [{ title: "Morning class" }],
      wsPrefix: "/app", filename: "classes", ...props,
    },
  });
  flushSync();
  return { target, done: () => { unmount(view); target.remove(); } };
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

  it("has no cell buttons without clickable columns", () => {
    const g = setup({ columns, rows });
    expect(g.target.querySelector("tbody button")).toBeNull();
    g.done();
  });
});
