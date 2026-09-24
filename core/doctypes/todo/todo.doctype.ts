import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "ToDo",
  module: "Core",
  label: "ToDo",
  icon: "check-square",
  titleField: "description",
  fields: [
    { fieldname: "status", fieldtype: "Select", label: "Status", options: ["Open", "Closed", "Cancelled"], optionColors: { Open: "blue", Closed: "green", Cancelled: "gray" }, default: "Open", inListView: true, inStandardFilter: true },
    { fieldname: "priority", fieldtype: "Select", label: "Priority", options: ["Low", "Medium", "High", "Urgent"], optionColors: { Low: "gray", Medium: "blue", High: "orange", Urgent: "red" }, default: "Medium", inListView: true, inStandardFilter: true },
    { fieldname: "date", fieldtype: "Date", label: "Due Date", inListView: true, inStandardFilter: true },
    { fieldname: "allocated_to", fieldtype: "Link", label: "Assigned To", options: "User", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "assigned_by", fieldtype: "Link", label: "Assigned By", options: "User", readOnly: true, inListView: true },
    { fieldname: "description", fieldtype: "Small Text", label: "Description" },
    { fieldname: "reference_type", fieldtype: "Data", label: "Reference Type", searchIndex: true },
    { fieldname: "reference_id", renamedFrom: "reference_name", fieldtype: "Dynamic Link", options: "reference_type", label: "Reference Document", searchIndex: true, inListView: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
    { role: "All", read: true, create: true, write: true, delete: true },
  ],
});
