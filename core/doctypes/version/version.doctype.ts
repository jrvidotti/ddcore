import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Version",
  module: "Core",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "ref_doctype", fieldtype: "Data", label: "DocType", searchIndex: true, inListView: true, inStandardFilter: true },
    { fieldname: "doc_id", renamedFrom: "docname", fieldtype: "Data", label: "Document", searchIndex: true, inListView: true, inStandardFilter: true },
    { fieldname: "data", fieldtype: "JSON", label: "Changes" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    // read access is filtered by the referenced document in the controller (B04)
    { role: "All", read: true },
  ],
});
