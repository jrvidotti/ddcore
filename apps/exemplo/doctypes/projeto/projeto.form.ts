import { defineForm, ddcore } from "@ddcore/desk-sdk";
import type { Projeto } from "../../.ddcore/types";

defineForm<Projeto>("Projeto", {
  refresh(frm) {
    if (frm.isNew) return;
    frm.addIndicator(__("Progress: {0}%", [ddcore.format.number(frm.doc.progresso, 2)]), corDoProgresso(frm.doc.progresso));
    frm.addButton(__("Tasks"), () => ddcore.route(`/app/Tarefa?projeto=${encodeURIComponent(frm.doc.name)}`));
  },
});

function corDoProgresso(progresso: number | null) {
  if (!progresso) return "gray";
  return progresso >= 100 ? "green" : "blue";
}
