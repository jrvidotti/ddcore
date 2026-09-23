import { describe, it, expect } from "vitest";
import { quickEntryFields, prefillField } from "./quick-create";
import type { DocTypeMeta } from "./meta";

const pessoa = (extra: any[] = []): DocTypeMeta => ({
  name: "Pessoa", app: "demo", label: "Pessoa", idGeneration: { by: "random" }, titleField: "nome",
  fields: [
    { fieldname: "nome", fieldtype: "Data", label: "Name" },
    { fieldname: "cpf", fieldtype: "Data", label: "CPF", reqd: true },
    { fieldname: "email", fieldtype: "Data", label: "Email" },
    { fieldname: "sec", fieldtype: "Section Break", label: "More" },
    { fieldname: "codigo", fieldtype: "Data", label: "Code", reqd: true, readOnly: true },
    { fieldname: "cidade", fieldtype: "Link", label: "City", reqd: true, hidden: true },
    { fieldname: "uf", fieldtype: "Data", label: "State", reqd: true, fetchFrom: "cidade.uf" },
    ...extra,
  ],
} as DocTypeMeta);

describe("quickEntryFields", () => {
  it("asks for the title and the required fields a person fills in", () => {
    expect(quickEntryFields(pessoa())!.map((f) => f.fieldname)).toEqual(["nome", "cpf"]);
  });

  it("skips an optional table and gives up on a required one", () => {
    expect(quickEntryFields(pessoa([{ fieldname: "tels", fieldtype: "Table", label: "Phones" }]))!.map((f) => f.fieldname)).toEqual(["nome", "cpf"]);
    expect(quickEntryFields(pessoa([{ fieldname: "tels", fieldtype: "Table", label: "Phones", reqd: true }]))).toBeNull();
  });

  it("prefills the title field, else the first Data field", () => {
    const d = pessoa();
    expect(prefillField(d, quickEntryFields(d)!)).toBe("nome");
    const untitled = { ...d, titleField: undefined };
    expect(prefillField(untitled, quickEntryFields(untitled)!)).toBe("cpf");
  });
});
