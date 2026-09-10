import "@cerne/sdk/test";
import type { Projeto, Tarefa } from "../../.cerne/types";

const u = () => cerne.utils;

function criarProjeto(valores: Partial<Projeto> = {}) {
  return cerne.newDoc<Projeto>("Projeto", {
    codigo: "P-" + u().randomString(6),
    titulo: "Projeto de teste",
    responsavel: "Administrator",
    data_inicio: u().today(),
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

describe("Projeto", () => {
  it("recusa data final anterior ao início", () => {
    expect(() =>
      criarProjeto({ data_inicio: u().addDays(u().today(), 1), data_final: u().today() })
    ).toThrow("final");
  });

  it("aceita data final igual ao início", () => {
    const p = criarProjeto({ data_final: u().today() });
    expect(p.data_final).toBe(u().today());
  });

  it("marco concluído exige a data de conclusão", () => {
    const p = cerne.newDoc<Projeto>("Projeto", {
      codigo: "P-" + u().randomString(6),
      titulo: "Com marco",
      responsavel: "Administrator",
      data_inicio: u().today(),
    });
    p.append("marcos", { titulo: "Entrega", data_prevista: u().today(), concluido: true });
    expect(() => p.insert()).toThrow();
  });

  it("marco reaberto limpa a data de conclusão", () => {
    const p = criarProjeto();
    p.append("marcos", { titulo: "Entrega", data_prevista: u().today(), concluido: true, concluido_em: u().today() });
    p.save();
    expect(p.marcos[0].concluido_em).toBe(u().today());

    p.marcos[0].concluido = false;
    p.save();
    expect(p.marcos[0].concluido_em).toBeNull();
  });

  it("sem tarefas: progresso 0 e status Planejado", () => {
    const p = criarProjeto();
    expect(p.progresso).toBe(0);
    expect(p.status).toBe("Planejado");
  });

  it("uma de duas concluídas: progresso 50 e status Em andamento", () => {
    const p = criarProjeto();
    const t = criarTarefa(p.name);
    criarTarefa(p.name);
    t.runMethod("concluir");

    p.reload();
    expect(p.progresso).toBe(50);
    expect(p.status).toBe("Em andamento");
  });

  it("todas concluídas: progresso 100 e status Concluído", () => {
    const p = criarProjeto();
    criarTarefa(p.name).runMethod("concluir");
    criarTarefa(p.name).runMethod("concluir");

    p.reload();
    expect(p.progresso).toBe(100);
    expect(p.status).toBe("Concluído");
  });
});
