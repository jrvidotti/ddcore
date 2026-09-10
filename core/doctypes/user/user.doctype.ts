import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "User",
  module: "Core",
  label: "Usuário",
  naming: { field: "email" },
  titleField: "full_name",
  searchFields: ["full_name"],
  trackChanges: true,
  icon: "user",
  fields: [
    { fieldname: "email", fieldtype: "Email", label: "E-mail", reqd: true, unique: true, inListView: true },
    { fieldname: "full_name", fieldtype: "Data", label: "Nome completo", reqd: true, inListView: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Ativo", default: true, inListView: true, inStandardFilter: true },
    { fieldtype: "Column Break" },
    { fieldname: "user_type", fieldtype: "Select", label: "Tipo", options: ["System User", "Website User"], default: "System User" },
    { fieldname: "language", fieldtype: "Data", label: "Idioma", default: "pt-BR" },
    { fieldname: "last_login", fieldtype: "Datetime", label: "Último acesso", readOnly: true },
    { fieldtype: "Section Break", label: "Senha" },
    { fieldname: "new_password", fieldtype: "Password", label: "Nova senha", description: "Preencha para definir ou trocar a senha." },
    { fieldname: "password_hash", fieldtype: "Data", label: "Hash", hidden: true, readOnly: true },
    { fieldtype: "Section Break", label: "Papéis" },
    { fieldname: "roles", fieldtype: "Table", label: "Papéis", options: "Has Role" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, export: true, report: true },
    { role: "All", read: true, ifOwner: true },
  ],
});
