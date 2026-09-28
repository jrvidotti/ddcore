import { defineController } from "@ddcore/sdk";

/** A version is only visible to those who can read the versioned document (B04). */
function canReadReference(doctype: string, docname: string, creation: unknown): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // deleted document
  // A delete keeps its Versions (#28). A document later created under the same
  // id is another document: reading it does not open the history of the one
  // that was deleted.
  const deletedSince = ddcore.db.getAll("Version", {
    filters: { ref_doctype: doctype, doc_id: docname, deleted: true, creation: [">=", creation] },
    fields: ["id"],
    limit: 1,
  });
  if (deletedSince.length > 0) return false;
  return ddcore.hasPermission(doctype, "read", { id: docname, owner }) === true;
}

export default defineController("Version", {
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // doctype verification: filter is applied per row
    if ((ddcore.getRoles(user) || []).indexOf("System Manager") >= 0) return true;
    // history is immutable for regular users
    if (ptype !== "read" && ptype !== "report" && ptype !== "export") return false;
    return canReadReference(String(doc.ref_doctype || ""), String(doc.doc_id || ""), doc.creation);
  },
});
