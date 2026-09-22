import { defineController, _ } from "@ddcore/sdk";

/** ToDo follows the read permission of the referenced document. Assignment never grants document access. */
function canReadReference(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // document deleted or not found
  return ddcore.hasPermission(doctype, "read", { id: docname, owner }) === true;
}

export default defineController("ToDo", {
  beforeInsert(doc) {
    // Only a System Manager may record a ToDo on someone else's behalf.
    const user = ddcore.user();
    const roles = ddcore.getRoles(user) || [];
    if (roles.indexOf("System Manager") < 0 || !doc.assigned_by) {
      doc.assigned_by = user;
    }
  },
  validate(doc) {
    const before = doc.getDocBeforeSave?.();
    if (!before) return;
    const roles = ddcore.getRoles(ddcore.user()) || [];
    if (roles.indexOf("System Manager") >= 0) return;
    // Who assigned what to whom changes only through the assignment endpoints;
    // otherwise an assignee could name someone else as assigner.
    for (const field of ["assigned_by", "allocated_to", "reference_type", "reference_id"] as const) {
      if (String(before[field] ?? "") !== String(doc[field] ?? "")) {
        ddcore.throw(_("{0} of a ToDo cannot be changed", [field]));
      }
    }
  },
  permissionQuery(user) {
    const roles = ddcore.getRoles(user) || [];
    if (roles.indexOf("System Manager") >= 0) return undefined;
    return { allocated_to: user };
  },
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // doctype verification: filter is applied per row
    const roles = ddcore.getRoles(user) || [];
    if (roles.indexOf("System Manager") >= 0) return true;
    if (user !== doc.allocated_to && user !== doc.assigned_by) return false;
    if (doc.reference_type && doc.reference_id) {
      if (!canReadReference(String(doc.reference_type), String(doc.reference_id))) {
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
