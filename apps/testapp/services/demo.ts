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

  const categories: { title: string; parent?: string; is_group?: boolean }[] = [
    { title: "Engineering", is_group: true },
    { title: "Frontend", parent: "Engineering" },
    { title: "Backend", parent: "Engineering" },
  ];

  for (const c of categories) {
    if (!ddcore.db.exists("Task Category", c.title)) {
      const cat = ddcore.newDoc("Task Category", {
        title: c.title,
        parent_task_category: c.parent || null,
        is_group: !!c.is_group,
      }).insert();
      created.push(cat.id);
    }
  }

  if (!ddcore.db.exists("Project", "DEMO")) {
    const project = ddcore.newDoc<Project>("Project", {
      code: "DEMO",
      title: "Demonstration project",
      description: "Created by `ddcore demo` to exercise the example app.",
      overview: "<h1>Demonstration Project</h1><p>This project exercises the <strong>ddcore</strong> framework capabilities, including rich text, field controls, and trees.</p>",
      assignee: "Admin",
      start_date: u().addDays(today, -30),
      end_date: u().addDays(today, 60),
    });
    project.append("milestones", { title: "Discovery", due_date: u().addDays(today, -15), completed: true, completed_on: u().addDays(today, -14) });
    project.append("milestones", { title: "Implementation", due_date: u().addDays(today, 20) });
    project.append("milestones", { title: "Delivery", due_date: u().addDays(today, 55) });
    project.insert();
    created.push(project.id);
  }

  const tasks: {
    code: string;
    title: string;
    priority: Task["priority"];
    days: number;
    action?: "start" | "complete";
    task_category?: string;
    estimated_duration?: number;
    complexity?: number;
    color?: string;
    notes?: string;
    snippet?: string;
    is_blocker?: boolean;
  }[] = [
    {
      code: "DEMO-01",
      title: "Write the scope",
      priority: "High",
      days: 7,
      task_category: "Frontend",
      estimated_duration: 7200,
      complexity: 3,
      color: "#2563eb",
      notes: "## Scope Checklist\n- [x] Initial design\n- [ ] User testing",
      snippet: "export function getScope() {\n  return { version: 1 };\n}",
      is_blocker: true,
    },
    {
      code: "DEMO-02",
      title: "Build the record form",
      priority: "Medium",
      days: 21,
      action: "start",
      task_category: "Frontend",
      estimated_duration: 14400,
      complexity: 4,
      color: "#16a34a",
      notes: "## Form Implementation\nRequires **Markdown** and **Code** components.",
      snippet: "import { defineForm } from '@ddcore/desk-sdk';",
      is_blocker: false,
    },
    {
      code: "DEMO-03",
      title: "Gather requirements",
      priority: "Low",
      days: -10,
      action: "complete",
      task_category: "Backend",
      estimated_duration: 3600,
      complexity: 2,
      color: "#ea580c",
      notes: "Requirements gathered from stakeholder interviews.",
      snippet: "interface Requirements {\n  readonly id: string;\n}",
      is_blocker: false,
    },
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
      task_category: t.task_category,
      estimated_duration: t.estimated_duration,
      complexity: t.complexity,
      color: t.color,
      notes: t.notes,
      snippet: t.snippet,
      is_blocker: t.is_blocker,
    }).insert();
    if (t.action) task.runMethod(t.action);
    created.push(task.id);
  }

  return { created, count: created.length };
}
