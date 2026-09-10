import { defineWorkspace, _ } from "@ddcore/sdk";
import { resumoPorStatus, rotuloStatus } from "../services/tarefas";

export default defineWorkspace({
  name: "Projetos",
  label: "Projetos",
  icon: "layout-dashboard",
  roles: ["System Manager", "Gestor de Projetos", "Colaborador de Projetos"],
  // a sidebar é sempre explícita: nunca inferida a partir dos DocTypes
  sidebar: [
    { label: "Visão Geral", route: "/app/workspace/Projetos", icon: "layout-dashboard" },
    { label: "Projetos", doctype: "Projeto", icon: "notepad-text" },
    { label: "Tarefas", doctype: "Tarefa", icon: "list" },
    { label: "Relatórios", icon: "bar-chart-3" },
    { label: "Tarefas por Status", report: "Tarefas por Status" },
  ],
  shortcuts: [
    { label: "Projetos", doctype: "Projeto", icon: "notepad-text" },
    { label: "Tarefas", doctype: "Tarefa", icon: "list" },
  ],
  numberCards: [
    { name: "projetos_em_andamento", label: "Projetos em Andamento", doctype: "Projeto", filters: { status: "Em andamento" }, color: "blue", route: "/app/Projeto?status=Em andamento" },
    { name: "tarefas_abertas", label: "Tarefas Abertas", doctype: "Tarefa", filters: { status: ["in", ["Aberta", "Em andamento"]] }, color: "green", route: "/app/Tarefa?status=Aberta" },
    { name: "tarefas_atrasadas", label: "Tarefas Atrasadas", doctype: "Tarefa", filters: { status: "Atrasada" }, color: "red", route: "/app/Tarefa?status=Atrasada" },
  ],
  charts: [
    {
      name: "tarefas_por_status",
      label: "Tarefas por Status",
      type: "bar",
      method() {
        // mesmo serviço do relatório: as contagens não podem divergir
        const resumo = resumoPorStatus();
        return {
          type: "bar" as const,
          labels: resumo.map((linha) => rotuloStatus(linha.status)),
          datasets: [{ name: _("Tasks"), values: resumo.map((linha) => linha.quantidade) }],
        };
      },
    },
  ],
  links: [
    { label: "Planejamento", items: [{ label: "Projetos", doctype: "Projeto" }] },
    { label: "Acompanhamento", items: [{ label: "Tarefas", doctype: "Tarefa" }, { label: "Tarefas por Status", report: "Tarefas por Status" }] },
  ],
});
