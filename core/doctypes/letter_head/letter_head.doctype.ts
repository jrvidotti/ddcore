import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "Letter Head",
  module: "Core",
  label: "Letter Head",
  idGeneration: { field: "letter_head_name" },
  allowRename: true,
  icon: "file-text",
  fields: [
    { fieldname: "letter_head_name", fieldtype: "Data", label: "Letter Head Name", reqd: true, unique: true, inListView: true },
    { fieldname: "is_default", fieldtype: "Check", label: "Is Default", default: false, inListView: true },
    { fieldname: "disabled", fieldtype: "Check", label: "Disabled", default: false, inListView: true },
    { fieldname: "align", fieldtype: "Select", label: "Align", options: ["Left", "Center", "Right"], default: "Left" },
    { fieldname: "image", fieldtype: "Attach", label: "Logo Image" },
    { fieldname: "header_html", fieldtype: "Text", label: "Header HTML" },
    { fieldname: "footer_html", fieldtype: "Text", label: "Footer HTML" },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true },
    { role: "All", read: true },
  ],
});
