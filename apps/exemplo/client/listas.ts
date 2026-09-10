import { defineListView } from "@cerne/desk-sdk";
import type { Tarefa } from "../.cerne/types";

const CORES: Record<string, string> = { Aberta: "blue", "Em andamento": "yellow", Atrasada: "red", Concluída: "green" };
const ROTULOS: Record<string, string> = { Aberta: "Open", "Em andamento": "In progress", Atrasada: "Overdue", Concluída: "Completed" };

defineListView<Tarefa>("Tarefa", {
  columns: ["projeto", "titulo", "responsavel", "prioridade", "status", "data_limite"],
  orderBy: "data_limite asc",
  indicator: (row) => {
    const status = row.status || "Aberta";
    return { label: __(ROTULOS[status] || status), color: CORES[status] || "gray" };
  },
});
