import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Role",
  module: "Core",
  label: "Role",
  naming: { field: "role_name" },
  allowRename: true,
  icon: "shield",
  fields: [
    { fieldname: "role_name", fieldtype: "Data", label: "Name", reqd: true, unique: true, inListView: true },
    { fieldname: "disabled", fieldtype: "Check", label: "Disabled" },
    { fieldname: "desk_access", fieldtype: "Check", label: "Desk access", default: true },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }, { role: "All", read: true }],
});
