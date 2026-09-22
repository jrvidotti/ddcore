import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import { type Meta } from "./meta";

vi.mock("./api", () => ({ api: { getDoc: vi.fn(), update: vi.fn(), insert: vi.fn() } }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = {
  doctype: { name: "Task", label: "Task", fields: [{ fieldname: "subject", fieldtype: "Data", reqd: true }], formApps: [] },
  permissions: { read: true, write: true, create: true },
} as unknown as Meta;

const saved = () => ({ doctype: "Task", id: "TASK-1", subject: "saved", modified: "2026-01-01 10:00:00" });

describe("restoring a draft", () => {
  it("puts the typed values back and leaves the form dirty", () => {
    const frm = new FormController(meta, saved());

    frm.applyDraft({ ...saved(), subject: "typed" });

    expect(frm.doc.subject).toBe("typed");
    expect(frm.isDirty).toBe(true);
  });

  it("forgets the errors the previous document had", () => {
    const frm = new FormController(meta, saved());
    frm.doc.subject = "";
    expect(frm.validateMandatory()).toBe(false);

    frm.applyDraft({ ...saved(), subject: "typed" });

    expect(frm.fieldErrors).toEqual({});
  });
});

describe("discarding changes", () => {
  it("goes back to the document as it was loaded, without asking the server", async () => {
    const frm = new FormController(meta, saved());
    frm.doc.subject = "typed";
    expect(frm.isDirty).toBe(true);

    await frm.discardChanges();

    expect(frm.doc.subject).toBe("saved");
    expect(frm.isDirty).toBe(false);
  });

  it("also undoes a restored draft", async () => {
    const frm = new FormController(meta, saved());
    frm.applyDraft({ ...saved(), subject: "typed" });

    await frm.discardChanges();

    expect(frm.doc.subject).toBe("saved");
    expect(frm.isDirty).toBe(false);
  });

  it("empties a new document back to the defaults it started with", async () => {
    const frm = new FormController(meta, { doctype: "Task", __islocal: true, subject: "" });
    frm.doc.subject = "typed";

    await frm.discardChanges();

    expect(frm.doc.subject).toBe("");
    expect(frm.isNew).toBe(true);
  });
});
