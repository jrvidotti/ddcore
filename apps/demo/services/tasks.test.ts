import "@ddcore/sdk/test";
import type { Project, Task } from "../.ddcore/types";
import { markOverdue, summaryByStatus } from "./tasks";

const u = () => ddcore.utils;

function makeProject(values: Partial<Project> = {}) {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Test project",
    assignee: "Administrator",
    start_date: u().addDays(u().today(), -60),
    ...values,
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

describe("markOverdue", () => {
  it("marks only the open ones that are past due, and returns the count", () => {
    const p = makeProject();
    const overdueOpen = makeTask(p.name, { due_date: u().addDays(u().today(), -1) });
    const overdueInProgress = makeTask(p.name, { due_date: u().addDays(u().today(), -5) });
    overdueInProgress.runMethod("start");
    const onTime = makeTask(p.name);
    const overdueCompleted = makeTask(p.name, { due_date: u().addDays(u().today(), -2) });
    overdueCompleted.runMethod("complete");

    expect(markOverdue()).toBe(2);

    expect(ddcore.db.getValue("Task", overdueOpen.name, "status")).toBe("Overdue");
    expect(ddcore.db.getValue("Task", overdueInProgress.name, "status")).toBe("Overdue");
    expect(ddcore.db.getValue("Task", onTime.name, "status")).toBe("Open");
    expect(ddcore.db.getValue("Task", overdueCompleted.name, "status")).toBe("Completed");
  });

  it("is idempotent: the second pass has nothing left to mark", () => {
    const p = makeProject();
    makeTask(p.name, { due_date: u().addDays(u().today(), -1) });
    expect(markOverdue()).toBe(1);
    expect(markOverdue()).toBe(0);
  });

  it("a task due today is not overdue", () => {
    const p = makeProject();
    makeTask(p.name, { due_date: u().today() });
    expect(markOverdue()).toBe(0);
  });
});

describe("summaryByStatus", () => {
  it("returns all four statuses with a count and a share", () => {
    const p = makeProject();
    makeTask(p.name);
    makeTask(p.name).runMethod("start");
    makeTask(p.name).runMethod("complete");
    makeTask(p.name).runMethod("complete");

    const summary = summaryByStatus({ project: p.name });
    expect(summary).toHaveLength(4);

    const byStatus: Record<string, { count: number; share: number }> = {};
    for (const row of summary) byStatus[row.status] = row;

    expect(byStatus["Open"].count).toBe(1);
    expect(byStatus["In progress"].count).toBe(1);
    expect(byStatus["Overdue"].count).toBe(0);
    expect(byStatus["Completed"].count).toBe(2);
    expect(byStatus["Completed"].share).toBe(50);
    expect(byStatus["Overdue"].share).toBe(0);
  });

  it("filters by assignee and by due date", () => {
    const p = makeProject();
    makeTask(p.name, { due_date: u().addDays(u().today(), 2) });
    makeTask(p.name, { due_date: u().addDays(u().today(), 40) });

    const upTo = summaryByStatus({ project: p.name, due_date_before: u().addDays(u().today(), 10) });
    expect(upTo.filter((row) => row.status === "Open")[0].count).toBe(1);

    const other = summaryByStatus({ project: p.name, assignee: "Guest" });
    expect(other.filter((row) => row.count > 0)).toHaveLength(0);
  });

  it("with no tasks, every share is zero", () => {
    const summary = summaryByStatus({ project: makeProject().name });
    expect(summary.filter((row) => row.share !== 0)).toHaveLength(0);
  });
});
