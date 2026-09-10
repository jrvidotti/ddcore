import { defineDoctype } from "@cerne/sdk";

export default defineDoctype({
  name: "API Key",
  module: "Core",
  label: "Chave de API",
  naming: { hash: true },
  fields: [
    { fieldname: "user", fieldtype: "Link", label: "Usuário", options: "User", reqd: true, inListView: true },
    { fieldname: "label", fieldtype: "Data", label: "Descrição", inListView: true },
    { fieldname: "secret_hash", fieldtype: "Data", label: "Hash do segredo", hidden: true, readOnly: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Ativa", default: true },
    { fieldname: "last_used", fieldtype: "Datetime", label: "Último uso", readOnly: true },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }],
});
