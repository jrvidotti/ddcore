import { defineDoctype } from "@ddcore/sdk";

// A customer organisation on a site with `tenancy` on. The documents are the
// site's own, not any tenant's: only the platform space reaches them.
export default defineDoctype({
  name: "Tenant",
  module: "Core",
  label: "Tenant",
  icon: "building-2",
  shared: true,
  idGeneration: { field: "slug" },
  titleField: "title",
  globalSearch: false,
  searchFields: ["slug", "title"],
  trackChanges: true,
  fields: [
    { fieldname: "slug", fieldtype: "Data", label: "Slug", reqd: true, unique: true, setOnlyOnce: true, inListView: true,
      description: "Lowercase letters, digits, hyphens and underscores. It cannot change." },
    { fieldname: "title", fieldtype: "Data", label: "Title", reqd: true, inListView: true },
    { fieldname: "enabled", fieldtype: "Check", label: "Enabled", default: true, inListView: true,
      description: "A disabled tenant refuses sign-ins, requests and background jobs." },
  ],
  permissions: [
    { role: "System Manager", read: true, write: true, create: true, delete: true, report: true, export: true },
  ],
});
