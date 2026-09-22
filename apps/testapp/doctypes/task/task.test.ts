import "@ddcore/sdk/test";
import type { Project, Task } from "../../.ddcore/types";

const u = () => ddcore.utils;

function makeProject(values: Partial<Project> = {}) {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Test project",
    assignee: "Admin",
    start_date: u().addDays(u().today(), -30),
    ...values,
  }).insert();
}

function makeTask(project: string, values: Partial<Task> = {}) {
  return ddcore.newDoc<Task>("Task", {
    code: "T-" + u().randomString(6),
    project,
    title: "Test task",
    assignee: "Admin",
    due_date: u().addDays(u().today(), 7),
    ...values,
  }).insert();
}

describe("Task", () => {
  it("is born Open", () => {
    const t = makeTask(makeProject().id);
    expect(t.status).toBe("Open");
    expect(t.completed_at).toBeNull();
  });

  it("rejects a due date before the project start", () => {
    const p = makeProject({ start_date: u().today() });
    expect(() => makeTask(p.id, { due_date: u().addDays(u().today(), -1) })).toThrow("due date cannot be earlier");
  });

  it("start is idempotent", () => {
    const t = makeTask(makeProject().id);
    expect(t.runMethod("start").status).toBe("In progress");
    expect(t.runMethod("start").status).toBe("In progress");
  });

  it("complete records the date and is idempotent", () => {
    const t = makeTask(makeProject().id);
    const r = t.runMethod("complete");
    expect(r.status).toBe("Completed");
    expect(r.completed_at).toBeTruthy();
    expect(t.runMethod("complete").completed_at).toBe(r.completed_at);
  });

  it("reopen clears the completion and goes back to Open", () => {
    const t = makeTask(makeProject().id);
    t.runMethod("complete");
    const r = t.runMethod("reopen");
    expect(r.status).toBe("Open");
    expect(r.completed_at).toBeNull();
    expect(t.runMethod("reopen").status).toBe("Open");
  });

  it("reopening a task whose due date has passed goes back to Overdue", () => {
    const p = makeProject();
    const t = makeTask(p.id, { due_date: u().addDays(u().today(), -3) });
    t.runMethod("complete");
    expect(t.runMethod("reopen").status).toBe("Overdue");
  });

  it("deleting a task recalculates the project", () => {
    const p = makeProject();
    const t = makeTask(p.id);
    makeTask(p.id).runMethod("complete");
    p.reload();
    expect(p.progress).toBe(50);

    t.delete();
    p.reload();
    expect(p.progress).toBe(100);
    expect(p.status).toBe("Completed");
  });

  it("moving a task recalculates both projects", () => {
    const origem = makeProject();
    const target = makeProject();
    const t = makeTask(origem.id);
    t.runMethod("complete");
    makeTask(origem.id);

    origem.reload();
    expect(origem.progress).toBe(50);

    t.set("project", target.id).save();

    origem.reload();
    target.reload();
    expect(origem.progress).toBe(0);
    expect(origem.status).toBe("In progress");
    expect(target.progress).toBe(100);
    expect(target.status).toBe("Completed");
  });

  it("stores and reads the new fieldtypes (DAT-08)", () => {
    const p = makeProject();
    const t = makeTask(p.id, {
      estimated_duration: 3600,
      complexity: 4,
      color: "#2563eb",
      notes: "## Requirements\n- item 1",
      snippet: "console.log('hello');",
      is_blocker: true,
    });

    t.reload();
    expect(t.estimated_duration).toBe(3600);
    expect(t.complexity).toBe(4);
    expect(t.color).toBe("#2563eb");
    expect(t.notes).toBe("## Requirements\n- item 1");
    expect(t.snippet).toBe("console.log('hello');");
    expect(t.is_blocker).toBe(true);
  });

  it("filters tasks by tree category branch (DAT-07)", () => {
    const rootCat = "Root Cat " + u().randomString(4);
    const childCat = "Child Cat " + u().randomString(4);

    ddcore.newDoc("Task Category", {
      title: rootCat,
      is_group: true,
    }).insert();

    ddcore.newDoc("Task Category", {
      title: childCat,
      parent_task_category: rootCat,
      is_group: false,
    }).insert();

    const p = makeProject();
    const t1 = makeTask(p.id, { task_category: childCat });
    const t2 = makeTask(p.id, { task_category: rootCat });

    const matched = ddcore.db.getList<Task>("Task", {
      filters: [["task_category", "descendants of (inclusive)", rootCat]],
    });

    const ids = matched.map((m) => m.id);
    expect(ids).toContain(t1.id);
    expect(ids).toContain(t2.id);
  });
});

