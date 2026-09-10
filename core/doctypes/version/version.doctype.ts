import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Version",
  module: "Core",
  label: "Versão",
  fields: [
    { fieldname: "ref_doctype", fieldtype: "Data", label: "DocType", searchIndex: true },
    { fieldname: "docname", fieldtype: "Data", label: "Documento", searchIndex: true },
    { fieldname: "data", fieldtype: "JSON", label: "Alterações" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    // leitura filtrada pelo documento referenciado no controller (B04)
    { role: "All", read: true },
  ],
});
