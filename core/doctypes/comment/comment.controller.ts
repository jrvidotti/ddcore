import { defineController } from "@ddcore/sdk";

/** Comment follows the read permission of the commented document (B04). */
function canReadReference(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // deleted document
  return ddcore.hasPermission(doctype, "read", { name: docname, owner }) === true;
}

export default defineController("Comment", {
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // doctype verification: filter is applied per row
    if ((ddcore.getRoles(user) || []).indexOf("System Manager") >= 0) return true;
    if (!canReadReference(String(doc.reference_doctype || ""), String(doc.reference_name || ""))) return false;
    // edit or delete, author only
    if (ptype === "write" || ptype === "delete") return doc.owner === user;
    return true;
  },
});
