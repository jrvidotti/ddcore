import "@ddcore/sdk/test";
import type { Projeto, Tarefa } from "../.ddcore/types";
import relatorio from "./tarefas_por_status.report";

const u = () => ddcore.utils;

function criarProjeto() {
  return ddcore.newDoc<Projeto>("Projeto", {
    codigo: "P-" + u().randomString(6),
    titulo: "Projeto de teste",
    responsavel: "Administrator",
    data_inicio: u().addDays(u().today(), -60),
  }).insert();
}

function criarTarefa(projeto: string, valores: Partial<Tarefa> = {}) {
  return ddcore.newDoc<Tarefa>("Tarefa", {
    codigo: "T-" + u().randomString(6),
    projeto,
    titulo: "Tarefa de teste",
    responsavel: "Administrator",
    data_limite: u().addDays(u().today(), 7),
    ...valores,
  }).insert();
}

describe("Relatório Tarefas por Status", () => {
  it("uma linha por status, com percentual e gráfico", () => {
    const p = criarProjeto();
    criarTarefa(p.name);
    criarTarefa(p.name).runMethod("concluir");

    const r = relatorio.execute({ projeto: p.name }, ddcore.session);

    expect(r.rows).toHaveLength(4);
    expect(r.columns.map((c) => c.fieldname)).toEqual(["status", "quantidade", "percentual"]);

    const aberta = r.rows.filter((l) => l.status === "Aberta")[0];
    expect(aberta.quantidade).toBe(1);
    expect(aberta.percentual).toBe(50);

    expect(r.chart!.type).toBe("bar");
    expect(r.chart!.labels).toHaveLength(4);
    expect(r.chart!.datasets[0].values.reduce((s, v) => s + v, 0)).toBe(2);
    expect(r.totals!.quantidade).toBe(2);
  });

  it("filtra por responsável e por data limite", () => {
    const p = criarProjeto();
    criarTarefa(p.name, { data_limite: u().addDays(u().today(), 2) });
    criarTarefa(p.name, { data_limite: u().addDays(u().today(), 40) });

    const ate = relatorio.execute({ projeto: p.name, data_limite_ate: u().addDays(u().today(), 10) }, ddcore.session);
    expect(ate.totals!.quantidade).toBe(1);

    const outro = relatorio.execute({ projeto: p.name, responsavel: "Guest" }, ddcore.session);
    expect(outro.totals!.quantidade).toBe(0);
  });
});
