import { defineDoctype } from "@cerne/sdk";

export default defineDoctype({
  name: "Role",
  module: "Core",
  label: "Papel",
  naming: { field: "role_name" },
  allowRename: true,
  icon: "shield",
  fields: [
    { fieldname: "role_name", fieldtype: "Data", label: "Nome", reqd: true, unique: true, inListView: true },
    { fieldname: "disabled", fieldtype: "Check", label: "Desativado" },
    { fieldname: "desk_access", fieldtype: "Check", label: "Acesso ao desk", default: true },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }, { role: "All", read: true }],
});
