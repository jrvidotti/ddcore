import { defineController, _ } from "@cerne/sdk";
import type { Projeto } from "../../.cerne/types";

export default defineController<Projeto>("Projeto", {
  validate(doc) {
    if (doc.data_final && doc.data_inicio && doc.data_final < doc.data_inicio) {
      cerne.throw(_("A data final não pode ser anterior à data de início."), { title: _("Datas inválidas") });
    }

    for (const marco of doc.marcos || []) {
      // marco reaberto não guarda data de conclusão; o inverso (concluído sem
      // data) o core já barra pelo mandatoryDependsOn do campo
      if (!marco.concluido) marco.concluido_em = null;
      if (marco.concluido_em && marco.data_prevista && marco.concluido_em < doc.data_inicio!) {
        cerne.throw(_("O marco {0} não pode ser concluído antes do início do projeto.", [marco.titulo]), {
          title: _("Marco inválido"),
        });
      }
    }

    // projeto novo nasce sem tarefas; daí em diante quem mantém os derivados é
    // services/projetos.ts, chamado pelos hooks da Tarefa
    if (doc.isNew()) {
      doc.progresso = 0;
      doc.status = "Planejado";
    }
  },
});
