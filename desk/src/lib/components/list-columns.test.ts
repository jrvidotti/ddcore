import { describe, expect, it } from "vitest";
import type { DocTypeMeta, Field } from "$lib/meta";
import { columnCandidates, columnsKey, defaultListColumns, exportFields, loadColumnChoice, resolveColumns, saveColumnChoice, toggleColumn } from "./list-columns";

const f = (fieldname: string, fieldtype = "Data", extra: Partial<Field> = {}): Field => ({ fieldname, fieldtype, label: fieldname, ...extra });

const organization: DocTypeMeta = {
  name: "Organization", app: "base", label: "Organization", idGeneration: { hash: true }, titleField: "legal_name",
  fields: [
    f("cnpj", "Data", { inListView: true }),
    f("legal_name", "Data", { inListView: true }),
    f("trade_name", "Data", { inListView: true }),
    f("contact_section", "Section Break"),
    f("phone"),
    f("city", "Link", { options: "City" }),
    f("state", "Link", { options: "State", hidden: true }),
    f("total", "Currency", { computed: true }),
    f("secret", "Password"),
    f("signature", "Signature"),
    f("contacts", "Table", { options: "Organization Contact" }),
  ],
};

const names = (fs: Field[]) => fs.map((x) => x.fieldname);

const memory = () => {
  const data = new Map<string, string>();
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
    removeItem: (k: string) => void data.delete(k),
  };
};

describe("list columns", () => {
  it("defaults to the app's columns", () => {
    expect(names(defaultListColumns(organization))).toEqual(["cnpj", "legal_name", "trade_name"]);
    expect(names(defaultListColumns(organization, { columns: ["phone", "cnpj"] as any }))).toEqual(["phone", "cnpj"]);
    const r = resolveColumns(organization, { modifiedColumn: false });
    expect(r).toMatchObject({ showID: false, showModified: false, customized: false });
  });

  it("offers the data fields a reader can see, never what has no column", () => {
    expect(names(columnCandidates(organization))).toEqual(["cnpj", "legal_name", "trade_name", "phone", "city"]);
    // a default column stays on offer, even one the rule would leave out
    expect(names(columnCandidates(organization, [organization.fields[6]]))).toContain("state");
  });

  it("hides Modified and adds Phone", () => {
    let r = resolveColumns(organization);
    let choice = toggleColumn(r, "modified", false);
    r = resolveColumns(organization, {}, choice);
    choice = toggleColumn(r, "phone", true);
    r = resolveColumns(organization, {}, choice);
    expect(names(r.columns)).toEqual(["cnpj", "legal_name", "trade_name", "phone"]);
    expect(r).toMatchObject({ showID: false, showModified: false, customized: true });
    r = resolveColumns(organization, {}, toggleColumn(r, "legal_name", false));
    expect(names(r.columns)).toEqual(["cnpj", "trade_name", "phone"]);
    r = resolveColumns(organization, {}, toggleColumn(r, "id", true));
    expect(r.showID).toBe(true);
  });

  it("drops a stored field the meta no longer offers", () => {
    const r = resolveColumns(organization, {}, { fields: ["phone", "gone", "secret", "total"] });
    expect(names(r.columns)).toEqual(["phone"]);
  });

  it("exports the visible columns in screen order", () => {
    const r = resolveColumns(organization, {}, { fields: ["phone", "cnpj"], id: true, modified: true });
    expect(exportFields(r)).toEqual(["id", "phone", "cnpj", "modified"]);
    expect(exportFields(resolveColumns(organization, { modifiedColumn: false }))).toEqual(["cnpj", "legal_name", "trade_name"]);
  });
});

describe("list column storage", () => {
  it("keys the choice by user, tenant and DocType", () => {
    expect(columnsKey("ana@x.com", "modaverao", "Organization")).toBe("ddcore_columns:ana@x.com:modaverao:Organization");
    expect(columnsKey(null, "", "Organization")).toBe("ddcore_columns:guest::Organization");
  });

  it("keeps a choice and deletes it on reset", () => {
    const store = memory();
    const key = columnsKey("ana", "t1", "Organization");
    saveColumnChoice(key, { fields: ["phone"], modified: false }, store);
    expect(loadColumnChoice(key, store)).toEqual({ fields: ["phone"], modified: false });
    expect(loadColumnChoice(columnsKey("ana", "t2", "Organization"), store)).toBeNull();
    saveColumnChoice(key, null, store);
    expect(store.data.size).toBe(0);
    expect(loadColumnChoice(key, store)).toBeNull();
  });

  it("ignores a stray value and a storage that throws", () => {
    const store = memory();
    store.setItem("k", "not json");
    expect(loadColumnChoice("k", store)).toBeNull();
    store.setItem("k", JSON.stringify({ fields: "phone" }));
    expect(loadColumnChoice("k", store)).toBeNull();
    store.setItem("k", JSON.stringify({ fields: ["phone", 3], id: "yes" }));
    expect(loadColumnChoice("k", store)).toEqual({ fields: ["phone"] });
    const broken = { getItem: () => { throw new Error("denied"); }, setItem: () => { throw new Error("denied"); }, removeItem: () => { throw new Error("denied"); } };
    expect(loadColumnChoice("k", broken)).toBeNull();
    expect(() => saveColumnChoice("k", { fields: [] }, broken)).not.toThrow();
    expect(() => saveColumnChoice("k", null, broken)).not.toThrow();
  });
});
