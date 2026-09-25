import { defineDoctype } from "@ddcore/sdk";

// A virtual DocType (DAT-07): no table, its rows are Projects and Tasks. Only
// what internal/acceptance asserts on — the union, a filter, a Link title,
// the refusals and a field masked by its source's permission level.
export default defineDoctype({
  name: "Work Item",
  module: "Projects",
  label: "Work Item",
  titleField: "title",
  searchFields: ["code", "title"],
  virtual: {
    sources: [
      { doctype: "Project", fields: { code: "code", title: "title", assignee: "assignee", budget: "budget" } },
      { doctype: "Task", fields: { code: "code", title: "title", assignee: "assignee" } },
    ],
  },
  fields: [
    { fieldname: "code", fieldtype: "Data", label: "Code", inListView: true },
    { fieldname: "title", fieldtype: "Data", label: "Title", inListView: true },
    { fieldname: "assignee", fieldtype: "Link", label: "Assignee", options: "User", inStandardFilter: true },
    // Project.budget sits at permission level 1: null for a contributor
    { fieldname: "budget", fieldtype: "Currency", label: "Budget" },
  ],
  permissions: [
    { role: "Project Manager", read: true, report: true, export: true },
    { role: "Project Contributor", read: true },
  ],
});
