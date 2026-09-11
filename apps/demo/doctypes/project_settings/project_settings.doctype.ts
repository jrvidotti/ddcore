import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Project Settings",
  label: "Project Settings",
  module: "Projects",
  isSingle: true,
  trackChanges: true,
  fields: [
    { fieldname: "planning_enabled", fieldtype: "Check", label: "Planning enabled", default: true },
    { fieldname: "notes", fieldtype: "Table", label: "Notes", options: "Project Setting Note" },
  ],
  permissions: [
    { role: "Project Manager", read: true, write: true },
    { role: "Project Contributor", read: true },
  ],
});
