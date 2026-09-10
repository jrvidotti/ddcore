import { defineController } from "@ddcore/sdk";

/** Uma versão só é visível para quem pode ler o documento versionado (B04). */
function podeLerReferencia(doctype: string, docname: string): boolean {
  if (!doctype || !docname) return false;
  const owner = ddcore.db.getValue(doctype, docname, "owner");
  if (owner === undefined || owner === null) return false; // documento apagado
  return ddcore.hasPermission(doctype, "read", { name: docname, owner }) === true;
}

export default defineController("Version", {
  hasPermission(doc, ptype, user) {
    if (!doc) return undefined; // verificação de doctype: o filtro é aplicado por linha
    if ((ddcore.getRoles(user) || []).indexOf("System Manager") >= 0) return true;
    // o histórico é imutável para usuários comuns
    if (ptype !== "read" && ptype !== "report" && ptype !== "export") return false;
    return podeLerReferencia(String(doc.ref_doctype || ""), String(doc.docname || ""));
  },
});
