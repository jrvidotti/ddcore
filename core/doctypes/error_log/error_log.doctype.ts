import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Error Log",
  module: "Core",
  label: "Error Log",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "method", fieldtype: "Data", label: "Source", inListView: true },
    { fieldname: "error", fieldtype: "Text", label: "Error", inListView: true },
    { fieldname: "seen", fieldtype: "Check", label: "Seen" },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, delete: true }],
});
