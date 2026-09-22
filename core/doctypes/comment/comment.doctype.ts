import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Comment",
  module: "Core",
  label: "Comment",
  icon: "message-square",
  fields: [
    { fieldname: "comment_type", fieldtype: "Select", label: "Type", options: ["Comment", "Info", "Like", "Attachment", "Workflow"], default: "Comment" },
    { fieldname: "reference_doctype", fieldtype: "Data", label: "DocType", searchIndex: true },
    { fieldname: "reference_id", renamedFrom: "reference_name", fieldtype: "Data", label: "Document", searchIndex: true },
    { fieldname: "content", fieldtype: "Text Editor", label: "Content" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    // read/create follow the referenced document; edit and delete, author only (B04)
    { role: "All", read: true, create: true },
    { role: "All", write: true, delete: true, ifOwner: true },
  ],
});
