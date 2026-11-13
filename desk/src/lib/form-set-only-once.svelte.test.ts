import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import type { Field, Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const tenant: Field = { fieldname: "tenant", fieldtype: "Link", label: "Tenant", options: "Tenant", setOnlyOnce: true };
const kind: Field = { fieldname: "kind", fieldtype: "Data", label: "Kind", setOnlyOnce: true };
const meta = {
  doctype: {
    name: "Member", label: "Member", formApps: [],
    fields: [tenant, { fieldname: "notes", fieldtype: "Table", label: "Notes", options: "Member Note" }],
  },
  permissions: { read: true, write: true },
} as unknown as Meta;

describe("setOnlyOnce", () => {
  it("is editable on a new document", () => {
    const f = new FormController(meta, { doctype: "Member", __islocal: true, tenant: "A" });
    expect(f.isFieldEditable(tenant)).toBe(true);
  });

  it("holds a saved value, whatever the form now shows", () => {
    const f = new FormController(meta, { doctype: "Member", id: "M-1", tenant: "A" });
    expect(f.isFieldEditable(tenant)).toBe(false);
    f.doc.tenant = null;
    expect(f.isFieldEditable(tenant)).toBe(false);
  });

  it("lets an empty saved value be filled once", () => {
    const f = new FormController(meta, { doctype: "Member", id: "M-1", tenant: null });
    f.setValue("tenant", "A");
    expect(f.isFieldEditable(tenant)).toBe(true);
    f.load({ doctype: "Member", id: "M-1", tenant: "A" });
    expect(f.isFieldEditable(tenant)).toBe(false);
  });

  it("keeps the header from offering a set titleField (#161)", () => {
    const titled = { ...meta, doctype: { ...meta.doctype, titleField: "tenant" } } as unknown as Meta;
    expect(new FormController(titled, { doctype: "Member", id: "M-1", tenant: "A" }).canEditTitleField).toBe(false);
    expect(new FormController(titled, { doctype: "Member", id: "M-1", tenant: null }).canEditTitleField).toBe(true);
    const editable = { ...titled, doctype: { ...titled.doctype, fields: [{ ...tenant, setOnlyOnce: false }] } } as unknown as Meta;
    expect(new FormController(editable, { doctype: "Member", id: "M-1", tenant: "A" }).canEditTitleField).toBe(true);
    const ro = { ...titled, doctype: { ...titled.doctype, fields: [{ ...tenant, setOnlyOnce: false, readOnly: true }] } } as unknown as Meta;
    expect(new FormController(ro, { doctype: "Member", id: "M-1", tenant: "A" }).canEditTitleField).toBe(false);
  });

  it("holds a saved row's value, not a new row's", () => {
    const f = new FormController(meta, { doctype: "Member", id: "M-1", notes: [{ id: "r1", kind: "k1" }, { id: "r2", kind: "" }] });
    f.addChild("notes", { kind: "k9" });
    const [r1, r2, fresh] = f.doc.notes;
    expect(f.isSetOnce(kind, r1, "notes")).toBe(true);
    expect(f.isSetOnce(kind, r2, "notes")).toBe(false);
    expect(f.isSetOnce(kind, fresh, "notes")).toBe(false);
  });
});
