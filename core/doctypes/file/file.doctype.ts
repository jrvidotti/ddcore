import { defineDoctype } from "@cerne/sdk";

export default defineDoctype({
  name: "File",
  module: "Core",
  label: "Arquivo",
  titleField: "file_name",
  icon: "paperclip",
  fields: [
    { fieldname: "file_name", fieldtype: "Data", label: "Nome", reqd: true, inListView: true },
    { fieldname: "file_url", fieldtype: "Data", label: "URL", reqd: true, unique: true, inListView: true },
    { fieldname: "file_size", fieldtype: "Int", label: "Tamanho" },
    { fieldname: "content_type", fieldtype: "Data", label: "Tipo" },
    { fieldname: "is_private", fieldtype: "Check", label: "Privado", default: true },
    { fieldtype: "Column Break" },
    { fieldname: "attached_to_doctype", fieldtype: "Data", label: "Anexado a (DocType)", searchIndex: true },
    { fieldname: "attached_to_name", fieldtype: "Data", label: "Anexado a (nome)", searchIndex: true },
    { fieldname: "attached_to_field", fieldtype: "Data", label: "Campo" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }, { role: "All", read: true, write: true, create: true, delete: true, ifOwner: true }],
});
