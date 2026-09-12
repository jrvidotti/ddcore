import { describe, expect, it, vi } from "vitest";
import { resolveFieldWidth, type Field, type Meta } from "./meta";
import { fieldSlots, LINE_SLOTS } from "./components/form-layout";
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

  it("defaults text, tables and HTML to full", () => {
    for (const ft of ["Text", "Small Text", "Text Editor", "JSON", "Table", "HTML"]) {
      expect(resolveFieldWidth({ fieldtype: ft })).toBe("full");
    }
  });

  it("defaults every other type to lg", () => {
    for (const ft of ["Data", "Email", "Link", "Dynamic Link", "Select", "Attach", "Password", "Check"]) {
      expect(resolveFieldWidth({ fieldtype: ft })).toBe("lg");
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
    expect(resolveFieldWidth({ fieldtype: "Data", width: "invalid" as any })).toBe("lg");
    expect(resolveFieldWidth({ fieldtype: "Text", width: "invalid" as any })).toBe("full");
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

  it("sizes cells in slots, a quarter of a form line each", () => {
    const row = LINE_SLOTS; // a form line
    const column = LINE_SLOTS / 2; // a dialog line, half as wide

    // sm and md take a quarter of the line either way
    for (const ft of ["Date", "Int", "Percent", "Datetime", "Currency", "Float"]) {
      expect(fieldSlots({ fieldtype: ft }, row)).toBe(1);
      expect(fieldSlots({ fieldtype: ft }, column)).toBe(1);
    }

    // lg takes half a line, and all of a narrow one
    for (const ft of ["Data", "Link", "Select", "Check"]) {
      expect(fieldSlots({ fieldtype: ft }, row)).toBe(2);
      expect(fieldSlots({ fieldtype: ft }, column)).toBe(2);
    }

    // full takes the line, clamped to the line it sits on
    expect(fieldSlots({ fieldtype: "Text" }, row)).toBe(4);
    expect(fieldSlots({ fieldtype: "Text" }, column)).toBe(2);

    // explicit overrides
    expect(fieldSlots({ fieldtype: "Data", width: "sm" }, row)).toBe(1);
    expect(fieldSlots({ fieldtype: "Date", width: "full" }, row)).toBe(4);
    expect(fieldSlots({ fieldtype: "Date", width: "lg" }, row)).toBe(2);
  });
});
