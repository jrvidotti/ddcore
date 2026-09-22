import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Task",
  module: "Projects",
  label: "Task",
  idGeneration: { field: "code" },
  titleField: "title",
  trackChanges: true,
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true, inListView: true },
    { fieldname: "project", fieldtype: "Link", label: "Project", options: "Project", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "description", fieldtype: "Text", label: "Description" },
    { fieldname: "assignee", fieldtype: "Link", label: "Assignee", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "priority", fieldtype: "Select", label: "Priority", options: ["Low", "Medium", "High"], default: "Medium", inListView: true },
    {
      fieldname: "status",
      fieldtype: "Select",
      label: "Status",
      options: ["Open", "In progress", "Overdue", "Completed"],
      // keyed by the canonical value, so the colour does not move with the
      // language; the desk uses this wherever it shows a status indicator
      optionColors: { Open: "blue", "In progress": "orange", Overdue: "red", Completed: "green" },
      readOnly: true,
      inListView: true,
    },
    { fieldname: "due_date", fieldtype: "Date", label: "Due date", reqd: true, inListView: true },
    { fieldname: "completed_at", fieldtype: "Datetime", label: "Completed at", readOnly: true },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Project Contributor", read: true, write: true, create: true, report: true },
  ],
});
