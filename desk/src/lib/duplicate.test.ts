import { describe, expect, it, vi } from "vitest";
import { duplicateDoc } from "./duplicate";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s, boot: {} }));

const meta = {
  doctype: {
    name: "Payout", label: "Payout",
    fields: [
      { fieldname: "amount", fieldtype: "Currency" },
      { fieldname: "e2e_id", fieldtype: "Data", unique: true },
      { fieldname: "status", fieldtype: "Select", readOnly: true, options: ["Pending", "Paid"], default: "Pending" },
      { fieldname: "supplier", fieldtype: "Link", options: "Supplier" },
      { fieldname: "supplier_name", fieldtype: "Data", readOnly: true, fetchFrom: "supplier.supplier_name" },
      { fieldname: "memo", fieldtype: "Small Text", noCopy: true },
      { fieldname: "urgent", fieldtype: "Check", noCopy: true },
      { fieldname: "amended_from", fieldtype: "Link", options: "Payout" },
      { fieldname: "sec", fieldtype: "Section Break" },
      { fieldname: "items", fieldtype: "Table", options: "Payout Item" },
      { fieldname: "log", fieldtype: "Table", options: "Payout Log", noCopy: true },
    ],
  },
  children: {
    "Payout Item": {
      name: "Payout Item", label: "Payout Item", isChild: true,
      fields: [
        { fieldname: "description", fieldtype: "Data" },
        { fieldname: "ref", fieldtype: "Data", noCopy: true, default: "-" },
      ],
    },
  },
  permissions: {},
} as unknown as Meta;

const original = {
  doctype: "Payout", id: "P-1", owner: "a@x", creation: "c", modified: "m", docstatus: 1,
  amount: 10, e2e_id: "E1", status: "Paid", supplier: "S1", supplier_name: "Acme",
  memo: "note", urgent: true, amended_from: "P-0", workflow_state: "Approved",
  items: [{ id: "r1", parent: "P-1", parentfield: "items", idx: 1, description: "one", ref: "X" }],
  log: [{ id: "l1", parent: "P-1" }],
};

describe("duplicateDoc", () => {
  const copy = duplicateDoc(meta, original);

  it("is a new draft with no identity", () => {
    expect(copy).toMatchObject({ doctype: "Payout", docstatus: 0, __islocal: true });
    for (const k of ["id", "owner", "creation", "modified", "workflow_state"]) expect(copy[k]).toBeUndefined();
    expect(copy.amended_from).toBeNull();
  });

  it("copies what a person types, and a fetchFrom that follows a kept Link", () => {
    expect(copy).toMatchObject({ amount: 10, supplier: "S1", supplier_name: "Acme" });
  });

  it("leaves unique, readOnly and noCopy fields at their defaults", () => {
    expect(copy).toMatchObject({ e2e_id: null, status: "Pending", memo: null, urgent: false });
  });

  it("copies child rows as new rows, honouring the child's noCopy", () => {
    expect(copy.items).toEqual([{ parentfield: "items", idx: 1, description: "one", ref: "-", docstatus: 0,
      id: undefined, parent: undefined, creation: undefined, modified: undefined, owner: undefined }]);
    expect(copy.log).toEqual([]);
  });

  it("leaves the original untouched", () => {
    expect(original.items[0].id).toBe("r1");
    expect(original.e2e_id).toBe("E1");
  });
});
