import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "../../meta";
import { showNameColumn } from "./name-column";

const doctype = (naming: any): DocTypeMeta => ({ name: "Company", app: "x", label: "Company", naming, titleField: "company_name", fields: [] });
const title: Field = { fieldname: "company_name", fieldtype: "Data", label: "Trade Name" };
const code: Field = { fieldname: "code", fieldtype: "Int", label: "Code" };

describe("showNameColumn", () => {
  it("shows the name by default", () => {
    expect(showNameColumn(doctype({ hash: true }), [title, code])).toBe(true);
  });
  it("leaves it out when the title field is the name and a column", () => {
    expect(showNameColumn(doctype({ field: "company_name" }), [title])).toBe(false);
    expect(showNameColumn(doctype({ field: "company_name" }), [code])).toBe(true);
  });
  it("honours nameColumn: false", () => {
    expect(showNameColumn(doctype({ hash: true }), [title, code], { nameColumn: false })).toBe(false);
    expect(showNameColumn(doctype({ hash: true }), [code], { nameColumn: false })).toBe(false);
  });
});
