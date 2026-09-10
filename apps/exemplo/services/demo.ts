// Dados de demonstração, descobertos por `ddcore demo`. Idempotente: consulta
// cada código antes de inserir, e os estados não iniciais saem dos métodos do
// controller — nunca de gravação direta nos campos derivados.
import type { Projeto, Tarefa } from "../.ddcore/types";

const u = () => ddcore.utils;

export interface ResultadoDemo {
  criados: string[];
  quantidade: number;
}

export function generate(): ResultadoDemo {
  const criados: string[] = [];
  const hoje = u().today();

  if (!ddcore.db.exists("Projeto", "DEMO")) {
    const projeto = ddcore.newDoc<Projeto>("Projeto", {
      codigo: "DEMO",
      titulo: "Projeto de demonstração",
      descricao: "Criado por `ddcore demo` para exercitar o app exemplo.",
      responsavel: "Administrator",
      data_inicio: u().addDays(hoje, -30),
      data_final: u().addDays(hoje, 60),
    });
    projeto.append("marcos", { titulo: "Levantamento", data_prevista: u().addDays(hoje, -15), concluido: true, concluido_em: u().addDays(hoje, -14) });
    projeto.append("marcos", { titulo: "Implementação", data_prevista: u().addDays(hoje, 20) });
    projeto.append("marcos", { titulo: "Entrega", data_prevista: u().addDays(hoje, 55) });
    projeto.insert();
    criados.push(projeto.name);
  }

  const tarefas: { codigo: string; titulo: string; prioridade: Tarefa["prioridade"]; dias: number; acao?: "iniciar" | "concluir" }[] = [
    { codigo: "DEMO-01", titulo: "Escrever o escopo", prioridade: "Alta", dias: 7 },
    { codigo: "DEMO-02", titulo: "Implementar o cadastro", prioridade: "Média", dias: 21, acao: "iniciar" },
    { codigo: "DEMO-03", titulo: "Levantar requisitos", prioridade: "Baixa", dias: -10, acao: "concluir" },
  ];

  for (const t of tarefas) {
    if (ddcore.db.exists("Tarefa", t.codigo)) continue;
    const tarefa = ddcore.newDoc<Tarefa>("Tarefa", {
      codigo: t.codigo,
      projeto: "DEMO",
      titulo: t.titulo,
      responsavel: "Administrator",
      prioridade: t.prioridade,
      data_limite: u().addDays(hoje, t.dias),
    }).insert();
    if (t.acao) tarefa.runMethod(t.acao);
    criados.push(tarefa.name);
  }

  return { criados, quantidade: criados.length };
}
