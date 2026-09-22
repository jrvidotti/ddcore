import { defineDoctype } from "@ddcore/sdk";

/**
 * One user's access to one document, granted by someone who could share it.
 *
 * Rows are written through `ddcore.share.*` and `/api/shares/*`, which check
 * the sharer's own rights; the permissions below only let a System Manager
 * inspect them. A share grants the document at permission level 0 and never
 * submit, cancel, delete or amend. See docs/agent/sharing.md.
 */
export default defineDoctype({
  name: "Document Share",
  module: "Core",
  label: "Document Share",
  icon: "share-2",
  idGeneration: { hash: true },
  titleField: "share_id",
  globalSearch: false,
  searchFields: ["user", "share_doctype", "share_id"],
  uniqueKeys: [{ name: "user_document", fields: ["user", "share_doctype", "share_id"] }],
  fields: [
    { fieldname: "user", fieldtype: "Link", label: "User", options: "User", reqd: true, inListView: true, inStandardFilter: true },
    { fieldname: "share_doctype", fieldtype: "Data", label: "Document Type", reqd: true, inListView: true, inStandardFilter: true, searchIndex: true },
    { fieldname: "share_id", renamedFrom: "share_name", fieldtype: "Data", label: "Document", reqd: true, inListView: true, searchIndex: true },
    { fieldname: "read", fieldtype: "Check", label: "Can Read", default: true, inListView: true },
    { fieldname: "write", fieldtype: "Check", label: "Can Write", default: false, inListView: true },
    { fieldname: "share", fieldtype: "Check", label: "Can Share", default: false, inListView: true },
    { fieldname: "override_scope", fieldtype: "Check", label: "Override Security Scope", default: false, inListView: true,
      description: "Reach the user even when the document is outside their User Permission scope." },
  ],
  permissions: [
    { role: "System Manager", read: true, report: true, export: true },
  ],
});
