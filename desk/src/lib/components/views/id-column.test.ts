import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "../../meta";
import { showIDColumn } from "./id-column";

const doctype = (idGeneration: any): DocTypeMeta => ({ name: "Company", app: "x", label: "Company", idGeneration, titleField: "company_name", fields: [] });
const title: Field = { fieldname: "company_name", fieldtype: "Data", label: "Trade Name" };
const code: Field = { fieldname: "code", fieldtype: "Int", label: "Code" };

describe("showIDColumn", () => {
  it("shows a meaningful id by default", () => {
    expect(showIDColumn(doctype({ series: "CO-.####" }), [title, code])).toBe(true);
    expect(showIDColumn(doctype({ prompt: true }), [title, code])).toBe(true);
  });
  it("hides a hash id by default", () => {
    expect(showIDColumn(doctype({ hash: true }), [title, code])).toBe(false);
    expect(showIDColumn(doctype({}), [code])).toBe(false);
    expect(showIDColumn(doctype(undefined), [code])).toBe(false);
  });
  it("keeps a hash id when the list has no other column", () => {
    expect(showIDColumn(doctype({ hash: true }), [])).toBe(true);
  });
  it("leaves it out when the title field is the id and a column", () => {
    expect(showIDColumn(doctype({ field: "company_name" }), [title])).toBe(false);
    expect(showIDColumn(doctype({ field: "company_name" }), [code])).toBe(true);
  });
  it("honours idColumn either way", () => {
    expect(showIDColumn(doctype({ series: "CO-.####" }), [title, code], { idColumn: false })).toBe(false);
    expect(showIDColumn(doctype({ hash: true }), [title, code], { idColumn: true })).toBe(true);
    expect(showIDColumn(doctype({ field: "company_name" }), [title], { idColumn: true })).toBe(true);
  });
});
