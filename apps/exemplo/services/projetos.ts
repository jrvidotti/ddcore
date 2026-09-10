// Progresso e status do Projeto são derivados das Tarefas: ninguém os digita.
// O controller de Tarefa chama este serviço depois de inserir, atualizar ou excluir.

export type StatusProjeto = "Planejado" | "Em andamento" | "Concluído";

export function recalcularProgresso(projeto: string): void {
  if (!projeto || !ddcore.db.exists("Projeto", projeto)) return;

  const tarefas = ddcore.db.getAll<{ status: string }>("Tarefa", {
    filters: { projeto },
    fields: ["status"],
    limit: 10000,
  });
  const concluidas = tarefas.filter((t) => t.status === "Concluída").length;
  const status: StatusProjeto =
    tarefas.length === 0 ? "Planejado" : concluidas === tarefas.length ? "Concluído" : "Em andamento";
  const progresso = tarefas.length === 0 ? 0 : ddcore.utils.roundTo((concluidas * 100) / tarefas.length, 2);

  // dbSet: grava as colunas derivadas sem reentrar no validate do Projeto
  ddcore.getDoc("Projeto", projeto).dbSet({ progresso, status });
}
