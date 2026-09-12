import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "File",
  module: "Core",
  label: "File",
  titleField: "file_name",
  icon: "paperclip",
  fields: [
    { fieldname: "file_name", fieldtype: "Data", label: "Name", reqd: true, inListView: true },
    { fieldname: "file_url", fieldtype: "Data", label: "URL", reqd: true, unique: true, inListView: true },
    { fieldname: "file_size", fieldtype: "Int", label: "Size" },
    { fieldname: "content_type", fieldtype: "Data", label: "Type" },
    { fieldname: "is_private", fieldtype: "Check", label: "Private", default: true },
    { fieldname: "attached_to_doctype", fieldtype: "Data", label: "Attached to (DocType)", searchIndex: true },
    { fieldname: "attached_to_name", fieldtype: "Data", label: "Attached to (name)", searchIndex: true },
    { fieldname: "attached_to_field", fieldtype: "Data", label: "Field" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }, { role: "All", read: true, write: true, create: true, delete: true, ifOwner: true }],
});
