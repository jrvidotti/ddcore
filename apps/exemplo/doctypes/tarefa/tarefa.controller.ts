import { defineController, _ } from "@cerne/sdk";
import type { Tarefa } from "../../.cerne/types";
import { recalcularProgresso } from "../../services/projetos";

/** Status derivado do prazo: usado ao criar e ao reabrir uma tarefa. */
function statusEmAberto(doc: Tarefa): "Aberta" | "Atrasada" {
  return doc.data_limite && doc.data_limite < cerne.utils.today() ? "Atrasada" : "Aberta";
}

function resultado(doc: Tarefa) {
  return { status: doc.status, concluida_em: doc.concluida_em };
}

export default defineController<Tarefa>("Tarefa", {
  beforeInsert(doc) {
    doc.status = "Aberta";
    doc.concluida_em = null;
  },

  validate(doc) {
    const inicio = cerne.db.getValue<string>("Projeto", doc.projeto!, "data_inicio");
    if (inicio && doc.data_limite && doc.data_limite < inicio) {
      cerne.throw(_("A data limite não pode ser anterior ao início do projeto ({0}).", [inicio]), {
        title: _("Prazo inválido"),
      });
    }
  },

  afterInsert(doc) {
    recalcularProgresso(doc.projeto!);
  },

  onUpdate(doc) {
    // ao mover a tarefa, o projeto de origem também muda de progresso
    const anterior = doc.getDocBeforeSave();
    if (anterior && anterior.projeto && anterior.projeto !== doc.projeto) recalcularProgresso(anterior.projeto);
    recalcularProgresso(doc.projeto!);
  },

  afterDelete(doc) {
    recalcularProgresso(doc.projeto!);
  },

  methods: {
    // as três transições são idempotentes e gravam pelo ciclo de vida normal
    iniciar(doc) {
      if (doc.status !== "Em andamento") {
        doc.status = "Em andamento";
        doc.concluida_em = null;
        doc.save();
      }
      return resultado(doc);
    },

    concluir(doc) {
      if (doc.status !== "Concluída") {
        doc.status = "Concluída";
        doc.concluida_em = cerne.utils.now();
        doc.save();
      }
      return resultado(doc);
    },

    reabrir(doc) {
      const destino = statusEmAberto(doc);
      if (doc.status !== destino || doc.concluida_em) {
        doc.status = destino;
        doc.concluida_em = null;
        doc.save();
      }
      return resultado(doc);
    },
  },
});
