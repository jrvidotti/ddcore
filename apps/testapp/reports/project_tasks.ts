import { defineReport, _ } from "@ddcore/sdk";

// A project's tasks, embedded in the Project form through a Report field.
export default defineReport({
  name: "Project Tasks",
  refDoctype: "Task",
  roles: ["Project Manager"],
  filters: [{ fieldname: "project", fieldtype: "Link", options: "Project", label: "Project", reqd: true }],
  execute(filters) {
    const rows = filters.project
      ? ddcore.db.getList("Task", { filters: { project: filters.project }, fields: ["id", "title", "status", "due_date"], orderBy: "due_date asc", limit: 1000 })
      : [];
    return {
      columns: [
        { fieldname: "id", label: _("Task"), fieldtype: "Link", options: "Task", width: 120 },
        { fieldname: "title", label: _("Title"), fieldtype: "Data", width: 200 },
        { fieldname: "status", label: _("Status"), fieldtype: "Select", width: 110 },
        { fieldname: "due_date", label: _("Due date"), fieldtype: "Date", width: 110 },
      ],
      rows,
    };
  },
});
