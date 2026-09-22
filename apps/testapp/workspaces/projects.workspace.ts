import { defineWorkspace, _ } from "@ddcore/sdk";
import { summaryByStatus } from "../services/tasks";

export default defineWorkspace({
  name: "Projects",
  label: "Projects",
  icon: "layout-dashboard",
  roles: ["System Manager", "Project Manager", "Project Contributor"],
  // the sidebar is always explicit: never inferred from the DocTypes
  sidebar: [
    { label: "Overview", route: "/app/workspace/Projects", icon: "layout-dashboard" },
    { label: "Projects", doctype: "Project", icon: "notepad-text" },
    { label: "Tasks", doctype: "Task", icon: "list" },
    { label: "Task Categories", doctype: "Task Category", icon: "folder-tree" },
  ],
  shortcuts: [
    { label: "Projects", doctype: "Project", icon: "notepad-text" },
    { label: "Tasks", doctype: "Task", icon: "list" },
    { label: "Task Categories", doctype: "Task Category", icon: "folder-tree" },
  ],
  numberCards: [
    { name: "open_tasks", label: "Open Tasks", doctype: "Task", filters: { status: ["in", ["Open", "In progress"]] }, color: "green", route: "/app/Task?status=Open" },
  ],
  charts: [
    {
      name: "tasks_by_status",
      label: "Tasks by Status",
      type: "bar",
      method() {
        const summary = summaryByStatus();
        return {
          type: "bar" as const,
          labels: summary.map((row) => _(row.status)),
          datasets: [{ name: _("Tasks"), values: summary.map((row) => row.count) }],
        };
      },
    },
  ],
  links: [
    {
      label: "Planning",
      items: [
        { label: "Projects", doctype: "Project" },
        { label: "Task Categories", doctype: "Task Category" },
      ],
    },
    { label: "Tracking", items: [{ label: "Tasks", doctype: "Task" }] },
  ],
});
