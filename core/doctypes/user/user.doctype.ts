import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "User",
  module: "Core",
  label: "User",
  naming: { field: "email" },
  titleField: "full_name",
  searchFields: ["full_name"],
  trackChanges: true,
  icon: "user",
  fields: [
    { fieldname: "email", fieldtype: "Email", label: "Email", reqd: true, unique: true, inListView: true },
    { fieldname: "full_name", fieldtype: "Data", label: "Full name", reqd: true, inListView: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true, inListView: true, inStandardFilter: true },
    { fieldtype: "Column Break" },
    { fieldname: "user_type", fieldtype: "Select", label: "Type", options: ["System User", "Website User"], default: "System User" },
    { fieldname: "language", fieldtype: "Data", label: "Language", description: "Leave blank to follow the site language." },
    { fieldname: "last_login", fieldtype: "Datetime", label: "Last login", readOnly: true },
    { fieldtype: "Section Break", label: "Password" },
    { fieldname: "new_password", fieldtype: "Password", label: "New password", description: "Fill this in to set or change the password." },
    { fieldname: "password_hash", fieldtype: "Data", label: "Hash", hidden: true, readOnly: true },
    { fieldtype: "Section Break", label: "Roles" },
    { fieldname: "roles", fieldtype: "Table", label: "Roles", options: "Has Role" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, export: true, report: true },
    { role: "All", read: true, ifOwner: true },
  ],
});
