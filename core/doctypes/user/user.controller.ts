import { defineController, _ } from "@ddcore/sdk";

export default defineController("User", {
  validate(doc) {
    doc.email = String(doc.email || "").trim().toLowerCase() === "admin" ? "Admin" : String(doc.email || "").trim();
    if (doc.new_password) {
      // __hashPassword applies the site's password policy and throws when the
      // password is too short — every path that sets one goes through it.
      doc.password_hash = (ddcore as any).__hashPassword(doc.new_password, doc.email || doc.name);
      doc.new_password = null;
    }
    const seen = new Set<string>();
    for (const r of doc.roles || []) {
      if (seen.has(r.role)) ddcore.throw(_("Role {0} is repeated", [r.role]));
      seen.add(r.role);
    }
  },
  onUpdate(doc) {
    ddcore.cache.del("roles:" + doc.name);
    // A changed password invalidates old sessions: if changed because the
    // password leaked, leaving existing sessions active would change nothing.
    const before = doc.getDocBeforeSave?.();
    if (before) {
      const oldRoles = new Set((before.roles || []).map((r: any) => String(r.role || "")));
      const newRoles = new Set((doc.roles || []).map((r: any) => String(r.role || "")));
      for (const r of newRoles) {
        if (r && !oldRoles.has(r)) {
          ddcore.audit("role.assign", "User", doc.name, { role: r });
        }
      }
      for (const r of oldRoles) {
        if (r && !newRoles.has(r)) {
          ddcore.audit("role.revoke", "User", doc.name, { role: r });
        }
      }
      if (Boolean(before.enabled) !== Boolean(doc.enabled)) {
        if (doc.enabled) {
          ddcore.audit("account.enable", "User", doc.name);
        } else {
          ddcore.audit("account.disable", "User", doc.name);
        }
      }
    } else {
      for (const r of doc.roles || []) {
        if (r && r.role) {
          ddcore.audit("role.assign", "User", doc.name, { role: String(r.role) });
        }
      }
    }
    if (before && before.password_hash !== doc.password_hash) {
      (ddcore as any).__dropSessions(doc.name);
    }
    // langFor serves the request language from this key without touching the
    // database, so changing User.language has to drop it.
    ddcore.cache.del("lang:" + doc.name);
    if (!doc.enabled) {
      (ddcore as any).__dropSessions(doc.name);
      // UserFromAPIKey already rejects keys for a disabled user, but the
      // row is cached for a minute: evict it now so the
      // deactivation takes effect immediately.
      for (const k of ddcore.db.getAll("API Key", { fields: ["name"], filters: { user: doc.name } })) {
        ddcore.cache.del("apikey:" + k.name);
      }
    }
  },
});
