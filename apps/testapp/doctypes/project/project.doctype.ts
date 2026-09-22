import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Project",
  module: "Projects",
  label: "Project",
  idGeneration: { field: "code" },
  titleField: "title",
  trackChanges: true,
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", reqd: true, unique: true, inListView: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "description", fieldtype: "Text", label: "Description" },
    { fieldname: "assignee", fieldtype: "Link", label: "Assignee", options: "User", reqd: true, inStandardFilter: true },
    {
      fieldname: "status",
      fieldtype: "Select",
      label: "Status",
      options: ["Planned", "In progress", "Completed"],
      optionColors: { Planned: "gray", "In progress": "orange", Completed: "green" },
      readOnly: true,
    },
    { fieldname: "start_date", fieldtype: "Date", label: "Start date", reqd: true },
    { fieldname: "end_date", fieldtype: "Date", label: "End date" },
    { fieldname: "progress", fieldtype: "Percent", label: "Progress", readOnly: true },
    // a field permission level: managers only (SEC-02, asserted in internal/acceptance)
    { fieldname: "budget", fieldtype: "Currency", label: "Budget", permlevel: 1 },
    { fieldname: "cover_image", fieldtype: "Attach Image", label: "Cover Image", description: "Project cover image" },
    { fieldname: "overview", fieldtype: "Text Editor", label: "Overview", description: "Rich text project overview" },
    { fieldname: "milestones", fieldtype: "Table", label: "Milestones", options: "Project Milestone", gridEditMode: "inline" },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "Project Contributor", read: true },
    { role: "Project Manager", permlevel: 1, read: true, write: true },
  ],
});
