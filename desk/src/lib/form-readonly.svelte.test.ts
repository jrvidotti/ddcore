import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = (permissions: Record<string, boolean>) =>
  ({ doctype: { name: "Bank", label: "Bank", fields: [{ fieldname: "code", fieldtype: "Data" }], formApps: [] }, permissions }) as unknown as Meta;

describe("a form without write permission", () => {
  it("keeps a saved document read-only", () => {
    const frm = new FormController(meta({ read: true, write: false, create: false }), { doctype: "Bank", id: "B-1", code: "001" });
    expect(frm.readOnly).toBe(true);
    expect(frm.isFieldEditable(frm.meta.doctype.fields[0])).toBe(false);
  });
  it("still fills a new document the user may create", () => {
    const frm = new FormController(meta({ read: true, write: false, create: true }), { doctype: "Bank", __islocal: true });
    expect(frm.readOnly).toBe(false);
    expect(frm.isFieldEditable(frm.meta.doctype.fields[0])).toBe(true);
  });
  it("edits a saved document with write", () => {
    const frm = new FormController(meta({ read: true, write: true }), { doctype: "Bank", id: "B-1" });
    expect(frm.readOnly).toBe(false);
    expect(frm.isFieldEditable(frm.meta.doctype.fields[0])).toBe(true);
  });
});
