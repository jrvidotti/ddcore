import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "API Key",
  module: "Core",
  label: "API Key",
  idGeneration: { hash: true },
  fields: [
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User", reqd: true, inListView: true },
    { fieldname: "label", fieldtype: "Data", label: "Description", inListView: true },
    { fieldname: "secret_hash", fieldtype: "Data", label: "Secret hash", hidden: true, readOnly: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true },
    { fieldname: "last_used", fieldtype: "Datetime", label: "Last used", readOnly: true },
    { fieldname: "expires", fieldtype: "Datetime", label: "Expires", description: "Leave blank for a key that never expires." },
  ],
  permissions: [{ role: "System Manager", read: true, write: true, create: true, delete: true }],
});
