import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Version",
  module: "Core",
  icon: "history",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "ref_doctype", fieldtype: "Data", label: "DocType", searchIndex: true, inListView: true, inStandardFilter: true },
    { fieldname: "doc_id", renamedFrom: "docname", fieldtype: "Data", label: "Document", searchIndex: true, inListView: true, inStandardFilter: true },
    // Set on the Version a delete writes: its data is {"deleted": <the document
    // as it was, children included>} instead of {"changed": ...}. It is how the
    // history of a deleted document ends, and how a System Manager finds what
    // was deleted (#28).
    { fieldname: "deleted", fieldtype: "Check", label: "Deleted", inListView: true, inStandardFilter: true, searchIndex: true },
    { fieldname: "data", fieldtype: "JSON", label: "Changes" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    // read access is filtered by the referenced document in the controller (B04)
    { role: "All", read: true },
  ],
});
