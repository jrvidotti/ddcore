import { defineController } from "@ddcore/sdk";

/** Comentário segue a permissão de leitura do documento comentado (B04). */
function podeLerReferencia(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // documento apagado
  return ddcore.hasPermission(doctype, "read", { name: docname, owner }) === true;
}

export default defineController("Comment", {
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // verificação de doctype: o filtro é aplicado por linha
    if ((ddcore.getRoles(user) || []).indexOf("System Manager") >= 0) return true;
    if (!podeLerReferencia(String(doc.reference_doctype || ""), String(doc.reference_name || ""))) return false;
    // alterar ou apagar, só o autor
    if (ptype === "write" || ptype === "delete") return doc.owner === user;
    return true;
  },
});
