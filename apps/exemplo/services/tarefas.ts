// Rotina de atraso e resumo por status. O resumo é compartilhado pelo relatório
// e pelo gráfico do workspace, para que as contagens não possam divergir.
import { whitelisted, _ } from "@cerne/sdk";
import type { Tarefa } from "../.cerne/types";

export const STATUS_TAREFA = ["Aberta", "Em andamento", "Atrasada", "Concluída"] as const;
export type StatusTarefa = (typeof STATUS_TAREFA)[number];

export interface FiltrosResumo {
  projeto?: string;
  responsavel?: string;
  data_limite_ate?: string;
}

export interface LinhaResumo {
  status: StatusTarefa;
  quantidade: number;
  percentual: number;
}

/**
 * Coloca em Atraso as tarefas cujo prazo passou e que não foram concluídas.
 * Grava pelo documento (e não por setValue) para que o ciclo de vida normal
 * recalcule o projeto. Uma falha isolada é registrada e não interrompe a rotina.
 */
export function marcarAtrasadas(): number {
  const pendentes = cerne.db.getAll<{ name: string }>("Tarefa", {
    filters: { status: ["in", ["Aberta", "Em andamento"]], data_limite: ["<", cerne.utils.today()] },
    fields: ["name"],
    limit: 10000,
  });

  let alteradas = 0;
  for (const linha of pendentes) {
    try {
      const tarefa = cerne.getDoc<Tarefa>("Tarefa", linha.name);
      tarefa.status = "Atrasada";
      tarefa.save();
      alteradas++;
    } catch (e) {
      cerne.log.error("Falha ao marcar a tarefa " + linha.name + " como atrasada: " + String(e));
    }
  }
  return alteradas;
}

/** Mesma regra do scheduler, disponível para execução manual no desk. */
export const marcarAtrasadasAgora = whitelisted(() => ({ alteradas: marcarAtrasadas() }), {
  roles: ["Gestor de Projetos"],
});

/** Uma linha por status, sempre as quatro, com o percentual sobre o total. */
export function resumoPorStatus(filtros: FiltrosResumo = {}): LinhaResumo[] {
  const condicoes: Record<string, any> = {};
  if (filtros.projeto) condicoes.projeto = filtros.projeto;
  if (filtros.responsavel) condicoes.responsavel = filtros.responsavel;
  if (filtros.data_limite_ate) condicoes.data_limite = ["<=", filtros.data_limite_ate];

  const tarefas = cerne.db.getList<{ status: string }>("Tarefa", {
    filters: condicoes,
    fields: ["status"],
    limit: 10000,
  });

  return STATUS_TAREFA.map((status) => {
    const quantidade = tarefas.filter((t) => t.status === status).length;
    return {
      status,
      quantidade,
      percentual: tarefas.length === 0 ? 0 : cerne.utils.roundTo((quantidade * 100) / tarefas.length, 2),
    };
  });
}

export const rotuloStatus = (status: StatusTarefa) =>
  ({ Aberta: _("Open"), "Em andamento": _("In progress"), Atrasada: _("Overdue"), Concluída: _("Completed") })[status];
