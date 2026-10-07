import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import { api } from "./api";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: { getSingle: vi.fn(), update: vi.fn(), insert: vi.fn() } }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

// a tool Single (#114) its reader fills on screen and never saves
const meta = (tool: boolean) => ({
  doctype: {
    name: "Link Generator", label: "Link Generator", isSingle: true, tool, formApps: [],
    fields: [
      { fieldname: "cpf", fieldtype: "Data" },
      { fieldname: "rows", fieldtype: "Table", options: "Link Generator Row" },
      { fieldname: "link", fieldtype: "Data", readOnly: true },
    ],
  },
  children: { "Link Generator Row": { name: "Link Generator Row", label: "Row", isChild: true, fields: [{ fieldname: "selected", fieldtype: "Check" }] } },
  permissions: { read: true, write: false },
}) as unknown as Meta;
const field = (frm: FormController, name: string) => frm.meta.doctype.fields.find((f) => f.fieldname === name)!;

describe("tool Singles", () => {
  it("lets a reader edit its fields and its grid", () => {
    const frm = new FormController(meta(true), { doctype: "Link Generator", id: "singleton", __islocal: true });
    expect(frm.isTool).toBe(true);
    expect(frm.readOnly).toBe(false);
    expect(frm.isFieldEditable(field(frm, "cpf"))).toBe(true);
    expect(frm.isFieldEditable(field(frm, "rows"))).toBe(true);
    // a field the DocType declares read-only stays so
    expect(frm.isFieldEditable(field(frm, "link"))).toBe(false);
  });

  it("is never dirty, whatever the script fills in", () => {
    const frm = new FormController(meta(true), { doctype: "Link Generator", id: "singleton", __islocal: true });
    frm.setValue("cpf", "52998224725");
    const row = frm.addChild("rows", { selected: 0 });
    frm.setRowValue("rows", row, "selected", 1);
    expect(frm.doc.rows[0].selected).toBe(1);
    expect(frm.isDirty).toBe(false);
  });

  it("never saves", async () => {
    const frm = new FormController(meta(true), { doctype: "Link Generator", id: "singleton", __islocal: true });
    frm.setValue("cpf", "52998224725");
    expect(await frm.save()).toBe(false);
    expect(api.update).not.toHaveBeenCalled();
    expect(api.insert).not.toHaveBeenCalled();
  });

  it("leaves a plain Single read-only for its reader", () => {
    const frm = new FormController(meta(false), { doctype: "Link Generator", id: "singleton", __islocal: true });
    expect(frm.isTool).toBe(false);
    expect(frm.readOnly).toBe(true);
    expect(frm.isFieldEditable(field(frm, "cpf"))).toBe(false);
  });
});
