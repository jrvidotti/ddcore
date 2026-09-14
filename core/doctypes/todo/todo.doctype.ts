import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "ToDo",
  module: "Core",
  label: "ToDo",
  icon: "check-square",
  fields: [
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Open", "Closed", "Cancelled"], default: "Open", inStandardFilter: true },
    { fieldname: "priority", fieldtype: "Select", label: "Priority", options: ["Low", "Medium", "High", "Urgent"], default: "Medium", inStandardFilter: true },
    { fieldname: "date", fieldtype: "Date", label: "Due Date", inStandardFilter: true },
    { fieldname: "allocated_to", fieldtype: "Link", label: "Assigned To", options: "User", reqd: true, inStandardFilter: true },
    { fieldname: "assigned_by", fieldtype: "Link", label: "Assigned By", options: "User", readOnly: true },
    { fieldname: "description", fieldtype: "Small Text", label: "Description" },
    { fieldname: "reference_type", fieldtype: "Data", label: "Reference DocType", searchIndex: true },
    { fieldname: "reference_name", fieldtype: "Data", label: "Reference Document", searchIndex: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "All", read: true, create: true, write: true, delete: true },
  ],
});
