import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = {
  doctype: {
    name: "Person", label: "Person", formApps: [],
    fields: [{ fieldname: "email", fieldtype: "Data", label: "Email" }],
  },
  permissions: { read: true, write: true },
} as unknown as Meta;

const frm = () => new FormController(meta, { doctype: "Person", id: "P-1" });

describe("field buttons", () => {
  it("keeps each button under the field it belongs to", () => {
    const f = frm();
    const onClick = vi.fn();
    f.addFieldButton("email", { label: "3 found", onClick });
    expect(f.fieldButtons.email).toHaveLength(1);
    expect(f.fieldButtons.phone).toBeUndefined();
    f.fieldButtons.email[0].onClick();
    expect(onClick).toHaveBeenCalled();
  });

  it("replaces the button with the same key, so a label carrying a count can be refreshed", () => {
    const f = frm();
    f.addFieldButton("email", { label: "1 found", onClick: () => {} });
    f.addFieldButton("email", { label: "3 found", onClick: () => {} });
    expect(f.fieldButtons.email.map((b) => b.label)).toEqual(["3 found"]);
  });

  it("keeps buttons with different keys side by side", () => {
    const f = frm();
    f.addFieldButton("email", { key: "lookup", label: "Look up", onClick: () => {} });
    f.addFieldButton("email", { key: "clear", label: "Clear", onClick: () => {} });
    expect(f.fieldButtons.email.map((b) => b.key)).toEqual(["lookup", "clear"]);
  });

  it("removes one button by key and the whole field without one", () => {
    const f = frm();
    f.addFieldButton("email", { key: "lookup", label: "Look up", onClick: () => {} });
    f.addFieldButton("email", { key: "clear", label: "Clear", onClick: () => {} });
    f.removeFieldButton("email", "lookup");
    expect(f.fieldButtons.email.map((b) => b.key)).toEqual(["clear"]);
    f.removeFieldButton("email");
    expect(f.fieldButtons.email).toBeUndefined();
  });

  it("drops the field entry once its last keyed button is removed", () => {
    const f = frm();
    f.addFieldButton("email", { key: "lookup", label: "Look up", onClick: () => {} });
    f.removeFieldButton("email", "lookup");
    expect(f.fieldButtons.email).toBeUndefined();
  });

  it("clears them with the toolbar buttons, so a refresh starts from scratch", () => {
    const f = frm();
    f.addFieldButton("email", { label: "3 found", onClick: () => {} });
    f.addButton("Do it", () => {});
    f.clearButtons();
    expect(f.fieldButtons).toEqual({});
    expect(f.buttons).toEqual([]);
  });
});
