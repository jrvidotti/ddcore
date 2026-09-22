import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "../../meta";
import { showIDColumn } from "./id-column";

const doctype = (idGeneration: any): DocTypeMeta => ({ name: "Company", app: "x", label: "Company", idGeneration, titleField: "company_name", fields: [] });
const title: Field = { fieldname: "company_name", fieldtype: "Data", label: "Trade Name" };
const code: Field = { fieldname: "code", fieldtype: "Int", label: "Code" };

describe("showIDColumn", () => {
  it("shows the id by default", () => {
    expect(showIDColumn(doctype({ hash: true }), [title, code])).toBe(true);
  });
  it("leaves it out when the title field is the id and a column", () => {
    expect(showIDColumn(doctype({ field: "company_name" }), [title])).toBe(false);
    expect(showIDColumn(doctype({ field: "company_name" }), [code])).toBe(true);
  });
  it("honours idColumn: false", () => {
    expect(showIDColumn(doctype({ hash: true }), [title, code], { idColumn: false })).toBe(false);
    expect(showIDColumn(doctype({ hash: true }), [code], { idColumn: false })).toBe(false);
  });
});
