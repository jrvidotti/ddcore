import "@ddcore/sdk/test";
import type { Project, Task } from "../../.ddcore/types";

const u = () => ddcore.utils;

function makeProject(values: Partial<Project> = {}) {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Test project",
    assignee: "Administrator",
    start_date: u().today(),
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

describe("Project", () => {
  it("rejects an end date before the start", () => {
    expect(() =>
      makeProject({ start_date: u().addDays(u().today(), 1), end_date: u().today() })
    ).toThrow("end date cannot be earlier");
  });

  it("accepts an end date equal to the start", () => {
    const p = makeProject({ end_date: u().today() });
    expect(p.end_date).toBe(u().today());
  });

  it("a completed milestone requires its completion date", () => {
    const p = ddcore.newDoc<Project>("Project", {
      code: "P-" + u().randomString(6),
      title: "Com milestone",
      assignee: "Administrator",
      start_date: u().today(),
    });
    p.append("milestones", { title: "Delivery", due_date: u().today(), completed: true });
    expect(() => p.insert()).toThrow();
  });

  it("a reopened milestone clears its completion date", () => {
    const p = makeProject();
    p.append("milestones", { title: "Delivery", due_date: u().today(), completed: true, completed_on: u().today() });
    p.save();
    expect(p.milestones[0].completed_on).toBe(u().today());

    p.milestones[0].completed = false;
    p.save();
    expect(p.milestones[0].completed_on).toBeNull();
  });

  it("sem tasks: progress 0 e status Planned", () => {
    const p = makeProject();
    expect(p.progress).toBe(0);
    expect(p.status).toBe("Planned");
  });

  it("one of two completed: progress 50 and status In progress", () => {
    const p = makeProject();
    const t = makeTask(p.name);
    makeTask(p.name);
    t.runMethod("complete");

    p.reload();
    expect(p.progress).toBe(50);
    expect(p.status).toBe("In progress");
  });

  it("all completed: progress 100 and status Completed", () => {
    const p = makeProject();
    makeTask(p.name).runMethod("complete");
    makeTask(p.name).runMethod("complete");

    p.reload();
    expect(p.progress).toBe(100);
    expect(p.status).toBe("Completed");
  });
});
