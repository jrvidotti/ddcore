import { defineDoctype } from "@ddcore/sdk";

// A hierarchy (DAT-07): the parent Link and `is_group` are added by the
// framework, so what the fixture declares is only what the acceptance suite
// asserts on.
export default defineDoctype({
  name: "Task Category",
  module: "Projects",
  label: "Task Category",
  isTree: true,
  idGeneration: { field: "title" },
  titleField: "title",
  fields: [{ fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true }],
  permissions: [
    { role: "System Manager", read: true, create: true, write: true, delete: true },
    { role: "Project Manager", read: true, create: true, write: true, delete: true },
    { role: "Project Contributor", read: true },
  ],
});
