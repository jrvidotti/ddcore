import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "User",
  module: "Core",
  label: "User",
  idGeneration: { field: "email" },
  titleField: "full_name",
  searchFields: ["full_name"],
  trackChanges: true,
  icon: "user",
  fields: [
    { fieldname: "email", fieldtype: "Email", label: "Email", reqd: true, unique: true, inListView: true },
    { fieldname: "full_name", fieldtype: "Data", label: "Full name", reqd: true, inListView: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true, inListView: true, inStandardFilter: true },
    { fieldname: "user_type", fieldtype: "Select", label: "Type", options: ["System User", "Website User"], default: "System User" },
    { fieldname: "language", fieldtype: "Select", label: "Language", description: "Leave blank to follow the site language." },
    { fieldname: "last_login", fieldtype: "Datetime", label: "Last login", readOnly: true },
    { fieldtype: "Section Break", label: "Password" },
    { fieldname: "new_password", fieldtype: "Password", label: "New password", description: "Fill this in to set or change the password." },
    { fieldname: "password_hash", fieldtype: "Data", label: "Hash", hidden: true, readOnly: true },
    { fieldtype: "Section Break", label: "Roles" },
    { fieldname: "roles", fieldtype: "Table", label: "Roles", options: "Has Role" },
  ],
  // An account is administered, never self-served: `ifOwner` would match
  // whoever *created* the row — the inviter, not its subject — so a grant to
  // `All` gave a user no record of their own and only made the doctype look
  // readable (an empty list instead of a refusal). What a person may do to
  // their own account goes through `core/services/profile.ts`.
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, export: true, report: true },
  ],
});
