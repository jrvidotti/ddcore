import { defineDoctype } from "@cerne/sdk";

export default defineDoctype({
  name: "Error Log",
  module: "Core",
  label: "Log de Erro",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "method", fieldtype: "Data", label: "Origem", inListView: true },
    { fieldname: "error", fieldtype: "Text", label: "Erro", inListView: true },
    { fieldname: "seen", fieldtype: "Check", label: "Visto" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, delete: true }],
});
