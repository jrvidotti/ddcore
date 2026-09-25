import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Project Milestone",
  module: "Projects",
  label: "Project Milestone",
  isChild: true,
  fields: [
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "due_date", fieldtype: "Date", label: "Due date", reqd: true, inListView: true },
    { fieldname: "completed", fieldtype: "Check", label: "Completed", default: false, inListView: true },
    { fieldname: "completed_on", fieldtype: "Date", label: "Completed on", mandatoryDependsOn: "doc.completed" },
    { fieldname: "overdue", fieldtype: "Check", label: "Overdue", computed: true, inListView: true },
  ],
});
