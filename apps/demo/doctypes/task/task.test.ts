import "@ddcore/sdk/test";
import type { Project, Task } from "../../.ddcore/types";

const u = () => ddcore.utils;

function makeProject(values: Partial<Project> = {}) {
  return ddcore.newDoc<Project>("Project", {
    code: "P-" + u().randomString(6),
    title: "Test project",
    assignee: "Administrator",
    start_date: u().addDays(u().today(), -30),
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

describe("Task", () => {
  it("nasce Open", () => {
    const t = makeTask(makeProject().name);
    expect(t.status).toBe("Open");
    expect(t.completed_at).toBeNull();
  });

  it("rejects a due date before the project start", () => {
    const p = makeProject({ start_date: u().today() });
    expect(() => makeTask(p.name, { due_date: u().addDays(u().today(), -1) })).toThrow("limite");
  });

  it("start is idempotent", () => {
    const t = makeTask(makeProject().name);
    expect(t.runMethod("start").status).toBe("In progress");
    expect(t.runMethod("start").status).toBe("In progress");
  });

  it("complete records the date and is idempotent", () => {
    const t = makeTask(makeProject().name);
    const r = t.runMethod("complete");
    expect(r.status).toBe("Completed");
    expect(r.completed_at).toBeTruthy();
    expect(t.runMethod("complete").completed_at).toBe(r.completed_at);
  });

  it("reopen clears the completion and goes back to Open", () => {
    const t = makeTask(makeProject().name);
    t.runMethod("complete");
    const r = t.runMethod("reopen");
    expect(r.status).toBe("Open");
    expect(r.completed_at).toBeNull();
    expect(t.runMethod("reopen").status).toBe("Open");
  });

  it("reopen task com prazo vencido volta para Overdue", () => {
    const p = makeProject();
    const t = makeTask(p.name, { due_date: u().addDays(u().today(), -3) });
    t.runMethod("complete");
    expect(t.runMethod("reopen").status).toBe("Overdue");
  });

  it("excluir a task recalcula o project", () => {
    const p = makeProject();
    const t = makeTask(p.name);
    makeTask(p.name).runMethod("complete");
    p.reload();
    expect(p.progress).toBe(50);

    t.delete();
    p.reload();
    expect(p.progress).toBe(100);
    expect(p.status).toBe("Completed");
  });

  it("mover a task recalcula os dois projects", () => {
    const origem = makeProject();
    const target = makeProject();
    const t = makeTask(origem.name);
    t.runMethod("complete");
    makeTask(origem.name);

    origem.reload();
    expect(origem.progress).toBe(50);

    t.set("project", target.name).save();

    origem.reload();
    target.reload();
    expect(origem.progress).toBe(0);
    expect(origem.status).toBe("In progress");
    expect(target.progress).toBe(100);
    expect(target.status).toBe("Completed");
  });
});
