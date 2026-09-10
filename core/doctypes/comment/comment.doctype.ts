import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Comment",
  module: "Core",
  label: "Comentário",
  icon: "message-square",
  fields: [
    { fieldname: "comment_type", fieldtype: "Select", label: "Tipo", options: ["Comment", "Info", "Like", "Attachment", "Workflow"], default: "Comment" },
    { fieldname: "reference_doctype", fieldtype: "Data", label: "DocType", searchIndex: true },
    { fieldname: "reference_name", fieldtype: "Data", label: "Documento", searchIndex: true },
    { fieldname: "content", fieldtype: "Text Editor", label: "Conteúdo" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    // leitura/criação seguem o documento referenciado; alterar e apagar, só o autor (B04)
    { role: "All", read: true, create: true },
    { role: "All", write: true, delete: true, ifOwner: true },
  ],
});
