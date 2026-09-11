import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Has Role",
  module: "Core",
  isChild: true,
  fields: [{ fieldname: "role", fieldtype: "Link", label: "Role", options: "Role", reqd: true, inListView: true }],
});
