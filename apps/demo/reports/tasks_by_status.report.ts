import { defineReport, _ } from "@ddcore/sdk";
import { summaryByStatus } from "../services/tasks";

export default defineReport({
  name: "Tasks by Status",
  label: "Tasks by Status",
  refDoctype: "Task",
  roles: ["System Manager", "Project Manager", "Project Contributor"],
  filters: [
    { fieldname: "project", label: "Project", fieldtype: "Link", options: "Project" },
    { fieldname: "assignee", label: "Assignee", fieldtype: "Link", options: "User" },
    { fieldname: "due_date_before", label: "Due date before", fieldtype: "Date" },
  ],
  execute(filters) {
    // the count comes from the same service the workspace chart uses
    const rows = summaryByStatus({
      project: filters.project,
      assignee: filters.assignee,
      due_date_before: filters.due_date_before,
    });
    const total = rows.reduce((sum, row) => sum + row.count, 0);

    return {
      columns: [
        { fieldname: "status", label: _("Status"), fieldtype: "Data", width: 160 },
        { fieldname: "count", label: _("Quantity"), fieldtype: "Int", width: 110 },
        { fieldname: "share", label: _("Share (%)"), fieldtype: "Percent", width: 120 },
      ],
      rows,
      totals: { status: _("Total"), count: total, share: total === 0 ? 0 : 100 },
      chart: {
        type: "bar",
        labels: rows.map((row) => _(row.status)),
        datasets: [{ name: _("Tasks"), values: rows.map((row) => row.count) }],
      },
    };
  },
});
