import { defineReport, _ } from "@cerne/sdk";
import { resumoPorStatus, rotuloStatus } from "../services/tarefas";

export default defineReport({
  name: "Tarefas por Status",
  label: "Tarefas por Status",
  refDoctype: "Tarefa",
  roles: ["System Manager", "Gestor de Projetos", "Colaborador de Projetos"],
  filters: [
    { fieldname: "projeto", label: "Projeto", fieldtype: "Link", options: "Projeto" },
    { fieldname: "responsavel", label: "Responsável", fieldtype: "Link", options: "User" },
    { fieldname: "data_limite_ate", label: "Data limite até", fieldtype: "Date" },
  ],
  execute(filters) {
    // a contagem vem do mesmo serviço usado pelo gráfico do workspace
    const rows = resumoPorStatus({
      projeto: filters.projeto,
      responsavel: filters.responsavel,
      data_limite_ate: filters.data_limite_ate,
    });
    const total = rows.reduce((soma, linha) => soma + linha.quantidade, 0);

    return {
      columns: [
        { fieldname: "status", label: _("Status"), fieldtype: "Data", width: 160 },
        { fieldname: "quantidade", label: _("Quantity"), fieldtype: "Int", width: 110 },
        { fieldname: "percentual", label: _("Share (%)"), fieldtype: "Percent", width: 120 },
      ],
      rows,
      totals: { status: _("Total"), quantidade: total, percentual: total === 0 ? 0 : 100 },
      chart: {
        type: "bar",
        labels: rows.map((linha) => rotuloStatus(linha.status)),
        datasets: [{ name: _("Tasks"), values: rows.map((linha) => linha.quantidade) }],
      },
    };
  },
});
