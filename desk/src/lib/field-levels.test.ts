import { describe, it, expect } from "vitest";
import { applyFieldLevels, type Meta } from "./meta";

const meta = (fieldLevels?: Meta["fieldLevels"]): Meta => ({
  doctype: {
    name: "Employee", app: "hr", label: "Employee", idGeneration: {},
    fields: [
      { fieldname: "title", fieldtype: "Data" },
      { fieldname: "salary", fieldtype: "Currency", permlevel: 1 },
      { fieldname: "review", fieldtype: "Small Text", permlevel: 2 },
      { fieldname: "lines", fieldtype: "Table", options: "Employee Line" },
    ],
  },
  children: {
    "Employee Line": {
      name: "Employee Line", app: "hr", label: "Employee Line", idGeneration: {}, isChild: true,
      fields: [
        { fieldname: "label", fieldtype: "Data" },
        { fieldname: "amount", fieldtype: "Currency", permlevel: 1 },
      ],
    },
  },
  permissions: { read: true, write: true },
  fieldLevels,
  series: null,
  linkTitles: {},
});

const names = (fields: { fieldname?: string }[]) => fields.map((f) => f.fieldname);

describe("applyFieldLevels", () => {
  it("removes unreadable fields from the DocType and its children", () => {
    const m = applyFieldLevels(meta({ read: [0], write: [0] }));
    expect(names(m.doctype.fields)).toEqual(["title", "lines"]);
    expect(names(m.children["Employee Line"].fields)).toEqual(["label"]);
  });

  it("makes readable but unwritable fields read-only", () => {
    const m = applyFieldLevels(meta({ read: [0, 1, 2], write: [0, 1] }));
    expect(names(m.doctype.fields)).toEqual(["title", "salary", "review", "lines"]);
    expect(m.doctype.fields.find((f) => f.fieldname === "review")?.readOnly).toBe(true);
    expect(m.doctype.fields.find((f) => f.fieldname === "salary")?.readOnly).toBeUndefined();
    expect(m.children["Employee Line"].fields.find((f) => f.fieldname === "amount")?.readOnly).toBeUndefined();
  });

  it("leaves a meta without levels untouched", () => {
    const raw = meta();
    expect(applyFieldLevels(raw)).toBe(raw);
  });
});
