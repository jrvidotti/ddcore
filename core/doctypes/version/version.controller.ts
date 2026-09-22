import { defineController } from "@ddcore/sdk";

/** A version is only visible to those who can read the versioned document (B04). */
function canReadReference(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // deleted document
  return ddcore.hasPermission(doctype, "read", { id: docname, owner }) === true;
}

export default defineController("Version", {
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // doctype verification: filter is applied per row
    if ((ddcore.getRoles(user) || []).indexOf("System Manager") >= 0) return true;
    // history is immutable for regular users
    if (ptype !== "read" && ptype !== "report" && ptype !== "export") return false;
    return canReadReference(String(doc.ref_doctype || ""), String(doc.doc_id || ""));
  },
});
