// Demonstration data, discovered by `ddcore demo`. Idempotent: it checks each
// code before inserting, and any state past the initial one comes from the
// controller's methods — never from writing a derived field directly.
//
// This is data, not interface: it is English and it is not in the catalogue.
import type { Project, Task } from "../.ddcore/types";

const u = () => ddcore.utils;

export interface DemoResult {
  created: string[];
  count: number;
}

export function generate(): DemoResult {
  const created: string[] = [];
  const today = u().today();

  if (!ddcore.db.exists("Project", "DEMO")) {
    const project = ddcore.newDoc<Project>("Project", {
      code: "DEMO",
      title: "Demonstration project",
      description: "Created by `ddcore demo` to exercise the example app.",
      assignee: "Admin",
      start_date: u().addDays(today, -30),
      end_date: u().addDays(today, 60),
    });
    project.append("milestones", { title: "Discovery", due_date: u().addDays(today, -15), completed: true, completed_on: u().addDays(today, -14) });
    project.append("milestones", { title: "Implementation", due_date: u().addDays(today, 20) });
    project.append("milestones", { title: "Delivery", due_date: u().addDays(today, 55) });
    project.insert();
    created.push(project.name);
  }

  const tasks: { code: string; title: string; priority: Task["priority"]; days: number; action?: "start" | "complete" }[] = [
    { code: "DEMO-01", title: "Write the scope", priority: "High", days: 7 },
    { code: "DEMO-02", title: "Build the record form", priority: "Medium", days: 21, action: "start" },
    { code: "DEMO-03", title: "Gather requirements", priority: "Low", days: -10, action: "complete" },
  ];

  for (const t of tasks) {
    if (ddcore.db.exists("Task", t.code)) continue;
    const task = ddcore.newDoc<Task>("Task", {
      code: t.code,
      project: "DEMO",
      title: t.title,
      assignee: "Admin",
      priority: t.priority,
      due_date: u().addDays(today, t.days),
    }).insert();
    if (t.action) task.runMethod(t.action);
    created.push(task.name);
  }

  return { created, count: created.length };
}
