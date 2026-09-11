import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Project Setting Note",
  label: "Project Setting Note",
  module: "Projects",
  isChild: true,
  fields: [
    { fieldname: "note", fieldtype: "Data", label: "Note", reqd: true, inListView: true },
  ],
});
