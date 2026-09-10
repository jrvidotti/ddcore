import { defineController, _ } from "@ddcore/sdk";

export default defineController("User", {
  validate(doc) {
    doc.email = String(doc.email || "").trim().toLowerCase() === "administrator" ? "Administrator" : String(doc.email || "").trim();
    if (doc.new_password) {
      doc.password_hash = (ddcore as any).__hashPassword(doc.new_password);
      doc.new_password = null;
    }
    const seen = new Set<string>();
    for (const r of doc.roles || []) {
      if (seen.has(r.role)) ddcore.throw(_("Papel {0} repetido", [r.role]));
      seen.add(r.role);
    }
  },
  onUpdate(doc) {
    ddcore.cache.del("roles:" + doc.name);
    if (!doc.enabled) {
      (ddcore as any).__dropSessions(doc.name);
      // UserFromAPIKey já recusa a chave de um usuário desativado, mas a
      // linha fica em cache por um minuto: derrube-a agora para que a
      // desativação valha imediatamente.
      for (const k of ddcore.db.getAll("API Key", { fields: ["name"], filters: { user: doc.name } })) {
        ddcore.cache.del("apikey:" + k.name);
      }
    }
  },
});
