import "@cerne/sdk/test";
import type { Projeto, Tarefa } from "../.cerne/types";
import { marcarAtrasadas, resumoPorStatus } from "./tarefas";

const u = () => cerne.utils;

function criarProjeto(valores: Partial<Projeto> = {}) {
  return cerne.newDoc<Projeto>("Projeto", {
    codigo: "P-" + u().randomString(6),
    titulo: "Projeto de teste",
    responsavel: "Administrator",
    data_inicio: u().addDays(u().today(), -60),
    ...valores,
  }).insert();
}

function criarTarefa(projeto: string, valores: Partial<Tarefa> = {}) {
  return cerne.newDoc<Tarefa>("Tarefa", {
    codigo: "T-" + u().randomString(6),
    projeto,
    titulo: "Tarefa de teste",
    responsavel: "Administrator",
    data_limite: u().addDays(u().today(), 7),
    ...valores,
  }).insert();
}

describe("marcarAtrasadas", () => {
  it("marca só as vencidas em aberto e devolve a quantidade", () => {
    const p = criarProjeto();
    const vencidaAberta = criarTarefa(p.name, { data_limite: u().addDays(u().today(), -1) });
    const vencidaEmAndamento = criarTarefa(p.name, { data_limite: u().addDays(u().today(), -5) });
    vencidaEmAndamento.runMethod("iniciar");
    const noPrazo = criarTarefa(p.name);
    const vencidaConcluida = criarTarefa(p.name, { data_limite: u().addDays(u().today(), -2) });
    vencidaConcluida.runMethod("concluir");

    expect(marcarAtrasadas()).toBe(2);

    expect(cerne.db.getValue("Tarefa", vencidaAberta.name, "status")).toBe("Atrasada");
    expect(cerne.db.getValue("Tarefa", vencidaEmAndamento.name, "status")).toBe("Atrasada");
    expect(cerne.db.getValue("Tarefa", noPrazo.name, "status")).toBe("Aberta");
    expect(cerne.db.getValue("Tarefa", vencidaConcluida.name, "status")).toBe("Concluída");
  });

  it("é idempotente: a segunda passada não tem o que marcar", () => {
    const p = criarProjeto();
    criarTarefa(p.name, { data_limite: u().addDays(u().today(), -1) });
    expect(marcarAtrasadas()).toBe(1);
    expect(marcarAtrasadas()).toBe(0);
  });

  it("tarefa no prazo de hoje não está atrasada", () => {
    const p = criarProjeto();
    criarTarefa(p.name, { data_limite: u().today() });
    expect(marcarAtrasadas()).toBe(0);
  });
});

describe("resumoPorStatus", () => {
  it("devolve os quatro status com quantidade e percentual", () => {
    const p = criarProjeto();
    criarTarefa(p.name);
    criarTarefa(p.name).runMethod("iniciar");
    criarTarefa(p.name).runMethod("concluir");
    criarTarefa(p.name).runMethod("concluir");

    const resumo = resumoPorStatus({ projeto: p.name });
    expect(resumo).toHaveLength(4);

    const porStatus: Record<string, { quantidade: number; percentual: number }> = {};
    for (const linha of resumo) porStatus[linha.status] = linha;

    expect(porStatus["Aberta"].quantidade).toBe(1);
    expect(porStatus["Em andamento"].quantidade).toBe(1);
    expect(porStatus["Atrasada"].quantidade).toBe(0);
    expect(porStatus["Concluída"].quantidade).toBe(2);
    expect(porStatus["Concluída"].percentual).toBe(50);
    expect(porStatus["Atrasada"].percentual).toBe(0);
  });

  it("filtra por responsável e por data limite", () => {
    const p = criarProjeto();
    criarTarefa(p.name, { data_limite: u().addDays(u().today(), 2) });
    criarTarefa(p.name, { data_limite: u().addDays(u().today(), 40) });

    const ate = resumoPorStatus({ projeto: p.name, data_limite_ate: u().addDays(u().today(), 10) });
    expect(ate.filter((l) => l.status === "Aberta")[0].quantidade).toBe(1);

    const outro = resumoPorStatus({ projeto: p.name, responsavel: "Guest" });
    expect(outro.filter((l) => l.quantidade > 0)).toHaveLength(0);
  });

  it("sem tarefas, todos os percentuais são zero", () => {
    const resumo = resumoPorStatus({ projeto: criarProjeto().name });
    expect(resumo.filter((l) => l.percentual !== 0)).toHaveLength(0);
  });
});
