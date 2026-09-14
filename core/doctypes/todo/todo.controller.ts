import { defineController } from "@ddcore/sdk";

/** ToDo follows the read permission of the referenced document. Assignment never grants document access. */
function canReadReference(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // document deleted or not found
  return ddcore.hasPermission(doctype, "read", { name: docname, owner }) === true;
}

export default defineController("ToDo", {
  beforeInsert(doc) {
    if (!doc.assigned_by) {
      doc.assigned_by = ddcore.user();
    }
  },
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // doctype verification: filter is applied per row
    const roles = ddcore.getRoles(user) || [];
    if (roles.indexOf("System Manager") >= 0) return true;
    if (user !== doc.allocated_to && user !== doc.assigned_by) return false;
    if (doc.reference_type && doc.reference_name) {
      if (!canReadReference(String(doc.reference_type), String(doc.reference_name))) {
        return false;
      }
    }
    if (ptype === "write") {
      return user === doc.allocated_to || user === doc.assigned_by;
    }
    if (ptype === "delete") {
      return user === doc.assigned_by;
    }
    return true;
  },
});
