import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Vault Audit Log",
  module: "Core",
  label: "Vault Audit Log",
  icon: "shield",
  sortField: "creation",
  sortOrder: "desc",
  fields: [
    { fieldname: "secret_name", fieldtype: "Data", label: "Secret Name", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "action", fieldtype: "Select", label: "Action", reqd: true, inListView: true, searchIndex: true, options: ["read", "write", "delete"] },
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User", inListView: true },
    { fieldname: "ip", fieldtype: "Data", label: "IP Address" },
    { fieldname: "request_id", fieldtype: "Data", label: "Request ID", inListView: true },
  ],
  permissions: [{ role: "System Manager", read: true, delete: true }],
});
