# Example Application Walkthrough (`ddcore-demo`)

The official example application, [ddcore-demo](https://github.com/jrvidotti/ddcore-demo), is an executable tutorial that demonstrates how real-world business entities, relational links, lifecycle hooks, and workspaces are structured in `ddcore` (*Data Driven Core*).

::: tip Live Interactive Demo
A public instance of `ddcore-demo` is deployed at `demo.ddcore.dev`:

👉 **[Launch Live Demo (demo.ddcore.dev)](https://demo.ddcore.dev)**

Sign in as **`visitor@example.com`** with the password **`demo-visitor`**. The visitor is a Project Manager: it can change projects, tasks, and invoices, but not users or system settings. Everything visitors change is wiped and seeded again every six hours.

A first walkthrough:

1. Open the **Projects** workspace: number cards, chart, and shortcuts.
2. Open project `PORTAL` and read its milestones and derived progress.
3. Start and complete one of its tasks, then watch the project's progress change.
4. Set a task's due date before its project starts, and see the server refuse it.
5. Open the **Tasks by Status** report, built on the same services.
6. Read the model, controller, and test behind each step in the [repository](https://github.com/jrvidotti/ddcore-demo).
:::

---

## 1. Overview of the Demo App

`ddcore-demo` models a lightweight project management and tracking tool:

- **Project:** The parent entity representing an initiative (with dates, budget, status, and calculated progress).
- **Task:** Individual work items assigned to users, linked to a Project, with due dates, priorities, and status transitions.
- **Project Milestone:** Child table within Project tracking intermediate deliverables and target dates.
- **Workspace:** An administrative dashboard showing active projects, overdue tasks, and progress distribution.

---

## 2. Running `ddcore-demo` Locally

Clone and start the example app in three commands:

```bash
git clone https://github.com/jrvidotti/ddcore-demo.git
cd ddcore-demo

# Ensure your PostgreSQL database is running (configured in ddcore.json)
ddcore migrate
ddcore user passwd Admin admin1234
ddcore dev
```

Navigate to `http://localhost:8092` (the port set in the demo's `ddcore.json`) and log in as `Admin`.

To populate the database with realistic sample projects, tasks, and users, seed the demo data:

```bash
ddcore demo
```

---

## 3. Relational Models & Child Tables

### Linking Documents (`Link` Fieldtype)
In `ddcore`, foreign key relationships are established using the `Link` fieldtype pointing to the target DocType.

In `task.doctype.ts`:
```typescript
{
  fieldname: "project",
  fieldtype: "Link",
  label: "Project",
  options: "Project",
  reqd: true,
  inListView: true,
  inStandardFilter: true,
}
```

This configuration:
1. Creates a foreign key column `project` in `tab_task` referencing `tab_project.name`.
2. Adds a search autocomplete input in the Task form.
3. Automatically enables filtering by Project in the Task list view.

### Embedded Child Tables (`Table` Fieldtype)
Child tables represent dependent line items stored in separate tables with parent pointers (`parent`, `parenttype`, `parentfield`, `idx`).

In `project.doctype.ts`:
```typescript
{
  fieldname: "milestones",
  fieldtype: "Table",
  label: "Milestones",
  options: "Project Milestone",
}
```

The Desk renders this as an embedded editable grid directly inside the Project form.

---

## 4. Business Logic Across Document Lifecycles

In `ddcore-demo`, completing a task automatically recalculates the parent project's overall completion percentage.

In `task.controller.ts`:
```typescript
import { defineController } from "@ddcore/sdk";
import type { Task } from "../../.ddcore/types";
import { recalculateProgress } from "../../services/projects";

export default defineController<Task>("Task", {
  afterInsert(doc) {
    recalculateProgress(doc.project!);
  },

  onUpdate(doc) {
    const previous = doc.getDocBeforeSave();
    if (previous && previous.project && previous.project !== doc.project) {
      // If task moved projects, recalculate previous project progress too
      recalculateProgress(previous.project);
    }
    recalculateProgress(doc.project!);
  },

  afterDelete(doc) {
    recalculateProgress(doc.project!);
  },
});
```

And in `services/projects.ts`:
```typescript
import { ddcore } from "@ddcore/sdk";

export function recalculateProgress(projectName: string) {
  const tasks = ddcore.db.getAll<{ status: string }>("Task", {
    filters: { project: projectName },
    fields: ["status"],
  });

  if (tasks.length === 0) {
    ddcore.db.setValues("Project", projectName, { progress: 0 });
    return;
  }

  const completed = tasks.filter((t) => t.status === "Completed").length;
  const progress = Math.round((completed / tasks.length) * 100);

  ddcore.db.setValues("Project", projectName, { progress });
}
```

Because this runs inside the single transaction initiated by the Task save, both the task update and project progress are committed atomically.

---

## 5. Client-Side Screen Scripts (`.form.ts`)

To provide immediate visual feedback before saving, `ddcore-demo` uses `@ddcore/desk-sdk`:

```typescript
// doctypes/task/task.form.ts
import { defineForm } from "@ddcore/desk-sdk";

export default defineForm({
  refresh(frm) {
    // Add custom action buttons on the form toolbar
    if (!frm.isNew() && frm.doc.status !== "Completed") {
      frm.addCustomButton("Mark as Done", () => {
        frm.call("complete").then(() => {
          frm.reload();
        });
      });
    }
  },
});
```

Form scripts run directly inside the browser's SvelteKit runtime and communicate with server methods defined in the controller's `methods` object.

---

## 6. What Next?

- Explore the full source code on [GitHub (jrvidotti/ddcore-demo)](https://github.com/jrvidotti/ddcore-demo).
- Learn how to run your application in staging or production in the [Deployment Guide](/guide/deployment).
