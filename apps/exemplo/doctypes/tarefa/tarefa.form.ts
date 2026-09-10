import { defineForm, cerne } from "@cerne/desk-sdk";
import type { Tarefa } from "../../.cerne/types";

const CORES: Record<string, string> = { Aberta: "blue", "Em andamento": "yellow", Atrasada: "red", Concluída: "green" };
const ROTULOS: Record<string, string> = { Aberta: "Open", "Em andamento": "In progress", Atrasada: "Overdue", Concluída: "Completed" };

defineForm<Tarefa>("Tarefa", {
  setup(frm) {
    // não se cria tarefa em projeto já concluído
    frm.setQuery("projeto", () => ({ filters: { status: ["!=", "Concluído"] } }));
  },
  refresh(frm) {
    if (frm.isNew) return;
    const status = frm.doc.status || "Aberta";
    frm.addIndicator(__(ROTULOS[status] || status), CORES[status] || "gray");

    // o estado só muda pelos métodos do controller; o form nunca grava derivados
    if (status !== "Em andamento" && status !== "Concluída") frm.addButton(__("Start"), () => transicao(frm, "iniciar"), __("Actions"));
    if (status !== "Concluída") frm.addButton(__("Complete"), () => transicao(frm, "concluir"), __("Actions"));
    if (status === "Concluída") frm.addButton(__("Reopen"), () => transicao(frm, "reabrir"), __("Actions"));
    frm.setInnerGroupAsPrimary(__("Actions"));
  },
});

async function transicao(frm: any, metodo: "iniciar" | "concluir" | "reabrir") {
  try {
    await frm.call(metodo);
    await frm.reload();
  } catch (e) {
    cerne.ui.showError(e);
  }
}
