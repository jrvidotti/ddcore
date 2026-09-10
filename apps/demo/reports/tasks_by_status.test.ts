import "@ddcore/sdk/test";
import type { Project, Task } from "../.ddcore/types";
import report from "./tasks_by_status.report";

const u = () => ddcore.utils;

function makeProject() {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Test project",
    assignee: "Administrator",
    start_date: u().addDays(u().today(), -60),
  }).insert();
}

function makeTask(project: string, values: Partial<Task> = {}) {
  return ddcore.newDoc<Task>("Task", {
    code: "T-" + u().randomString(6),
    project,
    title: "Test task",
    assignee: "Administrator",
    due_date: u().addDays(u().today(), 7),
    ...values,
  }).insert();
}

describe("Tasks by Status report", () => {
  it("one row per status, with a share and a chart", () => {
    const p = makeProject();
    makeTask(p.name);
    makeTask(p.name).runMethod("complete");

    const r = report.execute({ project: p.name }, ddcore.session);

    expect(r.rows).toHaveLength(4);
    expect(r.columns.map((c) => c.fieldname)).toEqual(["status", "count", "share"]);

    const open = r.rows.filter((row) => row.status === "Open")[0];
    expect(open.count).toBe(1);
    expect(open.share).toBe(50);

    expect(r.chart!.type).toBe("bar");
    expect(r.chart!.labels).toHaveLength(4);
    expect(r.chart!.datasets[0].values.reduce((s, v) => s + v, 0)).toBe(2);
    expect(r.totals!.count).toBe(2);
  });

  it("filters by assignee and by due date", () => {
    const p = makeProject();
    makeTask(p.name, { due_date: u().addDays(u().today(), 2) });
    makeTask(p.name, { due_date: u().addDays(u().today(), 40) });

    const upTo = report.execute({ project: p.name, due_date_before: u().addDays(u().today(), 10) }, ddcore.session);
    expect(upTo.totals!.count).toBe(1);

    const other = report.execute({ project: p.name, assignee: "Guest" }, ddcore.session);
    expect(other.totals!.count).toBe(0);
  });
});
