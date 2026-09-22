import { defineDoctype } from "@ddcore/sdk";

export default defineDoctype({
  name: "User Permission",
  module: "Core",
  label: "User Permission",
  icon: "shield",
  idGeneration: { hash: true },
  titleField: "for_value",
  globalSearch: false,
  searchFields: ["user", "allow", "for_value"],
  trackChanges: true,
  fields: [
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "allow", fieldtype: "Data", label: "DocType", reqd: true, inListView: true, inStandardFilter: true,
      description: "The DocType that defines the restricted scope, e.g. Company or Branch." },
    { fieldname: "for_value", fieldtype: "Data", label: "For Value", reqd: true, inListView: true, searchIndex: true,
      description: "The name/ID of the allowed document." },
    { fieldname: "applicable_for", fieldtype: "Data", label: "Applicable For", inListView: true,
      description: "Optional DocType to restrict this rule to. If empty, applies globally." },
    { fieldname: "is_default", fieldtype: "Check", label: "Is Default", default: false, inListView: true },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
  ],
});
