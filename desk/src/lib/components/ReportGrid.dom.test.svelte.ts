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
