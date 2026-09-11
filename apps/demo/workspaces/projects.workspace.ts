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
    { label: "Project Settings", doctype: "Project Settings", icon: "settings" },
    { label: "Reports", icon: "bar-chart-3" },
    { label: "Tasks by Status", report: "Tasks by Status" },
  ],
  shortcuts: [
    { label: "Projects", doctype: "Project", icon: "notepad-text" },
    { label: "Tasks", doctype: "Task", icon: "list" },
  ],
  numberCards: [
    { name: "projects_in_progress", label: "Projects in Progress", doctype: "Project", filters: { status: "In progress" }, color: "blue", route: "/app/Project?status=In progress" },
    { name: "open_tasks", label: "Open Tasks", doctype: "Task", filters: { status: ["in", ["Open", "In progress"]] }, color: "green", route: "/app/Task?status=Open" },
    { name: "overdue_tasks", label: "Overdue Tasks", doctype: "Task", filters: { status: "Overdue" }, color: "red", route: "/app/Task?status=Overdue" },
  ],
  charts: [
    {
      name: "tasks_by_status",
      label: "Tasks by Status",
      type: "bar",
      method() {
        // the report's own service: the counts cannot drift apart
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
    { label: "Planning", items: [{ label: "Projects", doctype: "Project" }] },
    { label: "Tracking", items: [{ label: "Tasks", doctype: "Task" }, { label: "Tasks by Status", report: "Tasks by Status" }] },
  ],
});
