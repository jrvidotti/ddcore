import { describe, expect, it, vi } from "vitest";
import { FormController, registerForm } from "./form.svelte";
import type { Meta } from "./meta";

vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = {
  doctype: {
    name: "Training Class", label: "Training Class", formApps: [],
    fields: [{ fieldname: "attendance", fieldtype: "Table", label: "Students", options: "Training Class Attendance" }],
  },
  permissions: { read: true, write: true },
} as unknown as Meta;

const onChange = vi.fn();
const clicks: string[] = [];
registerForm("Training Class", {
  onChange: { attendance: onChange },
  grids: { attendance: { onCellClick: { employee: (frm, row) => { clicks.push(`a:${row.employee}`); frm.setRowValue("attendance", row, "in_class", row.in_class ? 0 : 1); } } } },
});
registerForm("Training Class", { grids: { attendance: { onCellClick: { employee: (_frm, row) => { clicks.push(`b:${row.employee}`); } } } } });

const frm = () => new FormController(meta, {
  doctype: "Training Class", id: "TC-1",
  attendance: [
    { id: "r1", idx: 1, employee: "E-1", in_class: 0 },
    { id: "r2", idx: 2, employee: "E-2", in_class: 1 },
  ],
});

describe("setRowValue", () => {
  it("changes the row found by object or id, marks the form dirty and fires the table's onChange", () => {
    onChange.mockClear();
    const f = frm();
    const row = f.doc.attendance[0];
    f.setRowValue("attendance", row, "in_class", 1);
    expect(f.doc.attendance[0].in_class).toBe(1);
    expect(f.isDirty).toBe(true);
    expect(onChange).toHaveBeenCalledWith(f, "Training Class Attendance", "r1", f.doc.attendance[0], ["in_class"]);
    f.setRowValue("attendance", "r2", { in_class: 0, employee: "E-2", note: "late" });
    expect(onChange).toHaveBeenLastCalledWith(f, "Training Class Attendance", "r2", f.doc.attendance[1], ["in_class", "note"]);
    expect(f.doc.attendance[1]).toMatchObject({ in_class: 0, note: "late" });
    expect(onChange).toHaveBeenCalledTimes(2);
  });

  it("does nothing when the value is unchanged", () => {
    onChange.mockClear();
    const f = frm();
    f.setRowValue("attendance", "r2", "in_class", 1);
    expect(onChange).not.toHaveBeenCalled();
    expect(f.isDirty).toBe(false);
  });

  it("throws for a row that is not in the table", () => {
    const f = frm();
    expect(() => f.setRowValue("attendance", "nope", "in_class", 1)).toThrow(/nope/);
    expect(() => f.setRowValue("attendance", { employee: "E-9" }, "in_class", 1)).toThrow();
  });

  it("reaches a row not saved yet, which has no id", () => {
    onChange.mockClear();
    const f = frm();
    const row = f.addChild("attendance", { employee: "E-3" });
    f.setRowValue("attendance", row, "in_class", 1);
    expect(row.in_class).toBe(1);
    expect(onChange).toHaveBeenCalledWith(f, "Training Class Attendance", undefined, row, ["in_class"]);
  });
});

describe("grid cell clicks", () => {
  it("runs every script's handler for the cell, and none for other cells", () => {
    clicks.length = 0;
    const f = frm();
    expect(f.cellClickHandlers("attendance", "employee")).toHaveLength(2);
    expect(f.cellClickHandlers("attendance", "in_class")).toHaveLength(0);
    expect(f.cellClickHandlers("other", "employee")).toHaveLength(0);
    f.clickCell("attendance", "employee", f.doc.attendance[0]);
    expect(clicks).toEqual(["a:E-1", "b:E-1"]);
    expect(f.doc.attendance[0].in_class).toBe(1);
  });
});

describe("grid field changes", () => {
  const events: string[] = [];
  const scripts = {
    onChange: { attendance: (_f: any, _cdt: any, _cdn: any, row: any, changed: any) => events.push(`table:${row?.employee}:${changed?.join(",")}`) },
    grids: {
      attendance: {
        onChange: {
          in_class: (f: FormController, row: any) => {
            events.push(`in_class:${row.employee}`);
            if (!row.in_class) f.setRowValue("attendance", row, { attended: 0, grade: null });
          },
          attended: (f: FormController, row: any) => {
            events.push(`attended:${row.employee}`);
            if (row.attended && !row.in_class) f.setRowValue("attendance", row, "in_class", 1);
          },
          grade: (f: FormController, row: any) => {
            if (row.grade != null && !row.in_class) f.setRowValue("attendance", row, { in_class: 1, attended: 1 });
          },
        },
      },
    },
  };
  registerForm("Training Session", scripts);
  registerForm("Training Session", { grids: { attendance: { onChange: { in_class: (_f, row) => { events.push(`second:${row.employee}`); } } } } });
  const session = () => new FormController({ ...meta, doctype: { ...meta.doctype, name: "Training Session" } } as Meta, {
    doctype: "Training Session", id: "TS-1",
    attendance: [
      { id: "r1", idx: 1, employee: "E-1", in_class: 1, attended: 1, grade: 8 },
      { id: "r2", idx: 2, employee: "E-2", in_class: 0, attended: 0, grade: null },
    ],
  });

  it("runs every script's handler for each changed field, before the table's onChange", async () => {
    events.length = 0;
    const f = session();
    await f.trigger("attendance", "Training Class Attendance", "r1", f.doc.attendance[0], ["in_class", "note"]);
    expect(events).toEqual(["in_class:E-1", "second:E-1", "table:E-1:in_class,note"]);
  });

  it("tells apart unchecking in_class from checking attended on the same row state", () => {
    const f = session();
    f.setRowValue("attendance", "r1", "in_class", 0);
    expect(f.doc.attendance[0]).toMatchObject({ in_class: 0, attended: 0, grade: null });
    f.setRowValue("attendance", "r2", "attended", 1);
    expect(f.doc.attendance[1]).toMatchObject({ in_class: 1, attended: 1 });
  });

  it("a grade on a row not in class checks in_class and attended", () => {
    const f = session();
    f.setRowValue("attendance", "r2", "grade", 7);
    expect(f.doc.attendance[1]).toMatchObject({ in_class: 1, attended: 1, grade: 7 });
  });

  it("fires no field handler when no row is given", async () => {
    events.length = 0;
    await session().trigger("attendance");
    expect(events).toEqual(["table:undefined:undefined"]);
  });
});
