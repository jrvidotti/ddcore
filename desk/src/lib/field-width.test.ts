import { describe, expect, it, vi } from "vitest";
import { resolveFieldWidth, isFieldHalfWidth, type Field, type Meta } from "./meta";
import { FormController } from "./form.svelte";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

describe("field width resolution", () => {
  it("defaults Date, Month, Time, Int, Percent to sm", () => {
    for (const ft of ["Date", "Month", "Time", "Int", "Percent"]) {
      expect(resolveFieldWidth({ fieldtype: ft })).toBe("sm");
    }
  });

  it("defaults Datetime, Float, Currency to md", () => {
    for (const ft of ["Datetime", "Float", "Currency"]) {
      expect(resolveFieldWidth({ fieldtype: ft })).toBe("md");
    }
  });

  it("defaults text, link, select, check, and other fields to full", () => {
    const fullTypes = [
      "Data", "Email", "Link", "Dynamic Link", "Select", "Attach", "Password",
      "Text", "Small Text", "Text Editor", "JSON", "Table", "HTML", "Check",
    ];
    for (const ft of fullTypes) {
      expect(resolveFieldWidth({ fieldtype: ft })).toBe("full");
    }
  });

  it("honors explicit width overrides", () => {
    expect(resolveFieldWidth({ fieldtype: "Date", width: "full" })).toBe("full");
    expect(resolveFieldWidth({ fieldtype: "Data", width: "sm" })).toBe("sm");
    expect(resolveFieldWidth({ fieldtype: "Data", width: "md" })).toBe("md");
    expect(resolveFieldWidth({ fieldtype: "Currency", width: "lg" })).toBe("lg");
  });

  it("falls back to fieldtype default if width is invalid", () => {
    expect(resolveFieldWidth({ fieldtype: "Percent", width: "invalid" as any })).toBe("sm");
    expect(resolveFieldWidth({ fieldtype: "Data", width: "invalid" as any })).toBe("full");
  });

  it("always resolves to full inside grid", () => {
    expect(resolveFieldWidth({ fieldtype: "Date" }, true)).toBe("full");
    expect(resolveFieldWidth({ fieldtype: "Percent" }, true)).toBe("full");
    expect(resolveFieldWidth({ fieldtype: "Date", width: "sm" }, true)).toBe("full");
    expect(resolveFieldWidth({ fieldtype: "Currency", width: "md" }, true)).toBe("full");
  });

  it("reflects dynamic width updates via frm.setDfProperty", () => {
    const meta = {
      doctype: {
        name: "Invoice", label: "Invoice", formApps: [],
        fields: [
          { fieldname: "due_date", fieldtype: "Date", label: "Due Date" },
          { fieldname: "rate", fieldtype: "Percent", label: "Rate" },
        ],
      },
      permissions: { read: true, write: true },
    } as unknown as Meta;

    const frm = new FormController(meta, { doctype: "Invoice", name: "INV-1" });

    const f1 = frm.field("due_date");
    expect(f1).toBeDefined();
    expect(resolveFieldWidth(f1!)).toBe("sm");

    frm.setDfProperty("due_date", "width", "full");
    const f1Updated = frm.field("due_date");
    expect(resolveFieldWidth(f1Updated!)).toBe("full");

    frm.setDfProperty("rate", "width", "lg");
    const f2Updated = frm.field("rate");
    expect(resolveFieldWidth(f2Updated!)).toBe("lg");
  });

  it("identifies half-width (50%) fields correctly", () => {
    // sm and md are half-width
    expect(isFieldHalfWidth({ fieldtype: "Date" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Int" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Percent" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Datetime" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Currency" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Float" })).toBe(true);

    // full and lg are not half-width (including Check by default)
    expect(isFieldHalfWidth({ fieldtype: "Data" })).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Link" })).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Select" })).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Text" })).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Check" })).toBe(false);

    // explicit overrides
    expect(isFieldHalfWidth({ fieldtype: "Data", width: "sm" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Data", width: "md" })).toBe(true);
    expect(isFieldHalfWidth({ fieldtype: "Date", width: "full" })).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Date", width: "lg" })).toBe(false);

    // in grid is never half-width
    expect(isFieldHalfWidth({ fieldtype: "Date" }, true)).toBe(false);
    expect(isFieldHalfWidth({ fieldtype: "Date", width: "sm" }, true)).toBe(false);

    // null / undefined safety
    expect(isFieldHalfWidth(null)).toBe(false);
    expect(isFieldHalfWidth(undefined)).toBe(false);
  });
});
