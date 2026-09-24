import { describe, expect, it, vi } from "vitest";
import { applyDocTypeSelectors, docTypeChoices, dynamicLinksOf } from "./doctype-selector";
import { FormController } from "./form.svelte";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s, boot: { data: null } }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const doctypes = { Pessoa: { label: "Person" }, Task: { label: "Assignment" } };

const meta = () => ({
  doctype: {
    name: "ToDo", label: "ToDo", formApps: [],
    fields: [
      { fieldname: "reference_type", fieldtype: "Data", label: "Reference DocType" },
      { fieldname: "reference_id", fieldtype: "Dynamic Link", options: "reference_type", label: "Reference Document" },
      { fieldname: "note", fieldtype: "Data", label: "Note" },
    ],
  },
  children: {},
  permissions: { read: true, write: true },
}) as unknown as Meta;

describe("docTypeChoices", () => {
  it("sorts the DocTypes by label", () => {
    expect(docTypeChoices(doctypes)).toEqual({ options: ["Task", "Pessoa"], optionLabels: ["Assignment", "Person"] });
    expect(docTypeChoices(undefined)).toEqual({ options: [], optionLabels: [] });
  });
});

describe("applyDocTypeSelectors", () => {
  it("turns only the Dynamic Link's type field into a DocType Select", () => {
    const fields = applyDocTypeSelectors(meta(), doctypes).doctype.fields;
    expect(fields[0]).toMatchObject({ fieldtype: "Select", options: ["Task", "Pessoa"], optionLabels: ["Assignment", "Person"] });
    expect(fields[1].fieldtype).toBe("Dynamic Link");
    expect(fields[2].fieldtype).toBe("Data");
  });

  it("leaves a DocType without Dynamic Links as it was", () => {
    const m = meta();
    m.doctype.fields = m.doctype.fields.filter((f) => f.fieldtype !== "Dynamic Link");
    expect(applyDocTypeSelectors(m, doctypes).doctype).toBe(m.doctype);
  });
});

describe("changing the type field", () => {
  it("lists the links that depend on it", () => {
    expect(dynamicLinksOf(meta().doctype, "reference_type")).toEqual(["reference_id"]);
    expect(dynamicLinksOf(meta().doctype, "note")).toEqual([]);
  });

  it("clears the document it pointed at", () => {
    const frm = new FormController(meta(), { doctype: "ToDo", id: "T-1", reference_type: "Pessoa", reference_id: "Ana" });
    frm.setValue("reference_type", "Task");
    expect(frm.doc.reference_id).toBeNull();
  });

  it("keeps a document set together with its type", () => {
    const frm = new FormController(meta(), { doctype: "ToDo", id: "T-1" });
    frm.setValue({ reference_type: "Pessoa", reference_id: "Ana" });
    expect(frm.doc.reference_id).toBe("Ana");
  });
});
