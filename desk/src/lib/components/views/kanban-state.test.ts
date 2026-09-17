import { describe, expect, it } from "vitest";
import { canDragKanban, groupKanbanRows, kanbanColumns, moveKanbanRow } from "./kanban-state";

const rows = [
  { name: "T1", status: "Open" },
  { name: "T2", status: "Closed" },
  { name: "T3", status: "Legacy" },
  { name: "T4", status: null },
  { name: "T5", status: "Open" },
];

describe("kanban view", () => {
  it("uses the options with their labels, then unknown values, then the empty column", () => {
    const cols = kanbanColumns(rows, "status", { options: ["Open", "Working", "Closed"], labels: ["Aberto", "Em andamento", "Fechado"] });
    expect(cols).toEqual([
      { value: "Open", label: "Aberto", declared: true },
      { value: "Working", label: "Em andamento", declared: true },
      { value: "Closed", label: "Fechado", declared: true },
      { value: "Legacy", label: "Legacy", declared: false },
      { value: "", label: "", declared: false },
    ]);
  });

  it("lets configured columns set the order and subset", () => {
    const cols = kanbanColumns([{ name: "T1", status: "Open" }], "status", { options: ["Open", "Closed"], labels: ["Aberto", "Fechado"], columns: ["Closed", "Open"] });
    expect(cols.map((c) => [c.value, c.label])).toEqual([["Closed", "Fechado"], ["Open", "Aberto"]]);
  });

  it("groups rows per column in load order", () => {
    const groups = groupKanbanRows(rows, "status");
    expect(groups.get("Open")?.map((r) => r.name)).toEqual(["T1", "T5"]);
    expect(groups.get("")?.map((r) => r.name)).toEqual(["T4"]);
  });

  it("moves a card without mutating the loaded rows", () => {
    const moved = moveKanbanRow(rows, "T1", "status", "Closed");
    expect(moved).not.toBe(rows);
    expect(moved[0]).toEqual({ name: "T1", status: "Closed" });
    expect(rows[0].status).toBe("Open");
    expect(moveKanbanRow(rows, "T1", "status", "Open")).toBe(rows);
    expect(moveKanbanRow(rows, "missing", "status", "Open")).toBe(rows);
    expect(moveKanbanRow(rows, "T2", "status", "")[1].status).toBeNull();
  });

  it("drags only drafts of a writable field", () => {
    expect(canDragKanban({ docstatus: 0 }, true)).toBe(true);
    expect(canDragKanban({}, true)).toBe(true);
    expect(canDragKanban({ docstatus: 1 }, true)).toBe(false);
    expect(canDragKanban({ docstatus: 0 }, false)).toBe(false);
  });
});
