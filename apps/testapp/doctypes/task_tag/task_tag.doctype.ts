import { defineDoctype } from "@ddcore/sdk";

// The row behind Task's `tags`, a Table MultiSelect: one Link, which is the
// value the desk shows as a pill.
export default defineDoctype({
  name: "Task Tag",
  module: "Projects",
  label: "Task Tag",
  isChild: true,
  fields: [
    { fieldname: "category", fieldtype: "Link", label: "Category", options: "Task Category", reqd: true },
  ],
});
