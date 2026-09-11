import { defineController, _ } from "@ddcore/sdk";

export default defineController("User", {
  validate(doc) {
    doc.email = String(doc.email || "").trim().toLowerCase() === "administrator" ? "Administrator" : String(doc.email || "").trim();
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
    // Uma senha trocada invalida as sessões antigas: se a troca foi porque a
    // senha vazou, deixar as sessões de pé não teria trocado nada.
    const before = doc.getDocBeforeSave?.();
    if (before && before.password_hash !== doc.password_hash) {
      (ddcore as any).__dropSessions(doc.name);
    }
    // langFor serves the request language from this key without touching the
    // database, so changing User.language has to drop it.
    ddcore.cache.del("lang:" + doc.name);
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
