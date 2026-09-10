import "@cerne/sdk/test";
import type { Projeto, Tarefa } from "../../.cerne/types";

const u = () => cerne.utils;

function criarProjeto(valores: Partial<Projeto> = {}) {
  return cerne.newDoc<Projeto>("Projeto", {
    codigo: "P-" + u().randomString(6),
    titulo: "Projeto de teste",
    responsavel: "Administrator",
    data_inicio: u().addDays(u().today(), -30),
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

describe("Tarefa", () => {
  it("nasce Aberta", () => {
    const t = criarTarefa(criarProjeto().name);
    expect(t.status).toBe("Aberta");
    expect(t.concluida_em).toBeNull();
  });

  it("recusa data limite anterior ao início do projeto", () => {
    const p = criarProjeto({ data_inicio: u().today() });
    expect(() => criarTarefa(p.name, { data_limite: u().addDays(u().today(), -1) })).toThrow("limite");
  });

  it("iniciar é idempotente", () => {
    const t = criarTarefa(criarProjeto().name);
    expect(t.runMethod("iniciar").status).toBe("Em andamento");
    expect(t.runMethod("iniciar").status).toBe("Em andamento");
  });

  it("concluir registra a data e é idempotente", () => {
    const t = criarTarefa(criarProjeto().name);
    const r = t.runMethod("concluir");
    expect(r.status).toBe("Concluída");
    expect(r.concluida_em).toBeTruthy();
    expect(t.runMethod("concluir").concluida_em).toBe(r.concluida_em);
  });

  it("reabrir limpa a conclusão e volta para Aberta", () => {
    const t = criarTarefa(criarProjeto().name);
    t.runMethod("concluir");
    const r = t.runMethod("reabrir");
    expect(r.status).toBe("Aberta");
    expect(r.concluida_em).toBeNull();
    expect(t.runMethod("reabrir").status).toBe("Aberta");
  });

  it("reabrir tarefa com prazo vencido volta para Atrasada", () => {
    const p = criarProjeto();
    const t = criarTarefa(p.name, { data_limite: u().addDays(u().today(), -3) });
    t.runMethod("concluir");
    expect(t.runMethod("reabrir").status).toBe("Atrasada");
  });

  it("excluir a tarefa recalcula o projeto", () => {
    const p = criarProjeto();
    const t = criarTarefa(p.name);
    criarTarefa(p.name).runMethod("concluir");
    p.reload();
    expect(p.progresso).toBe(50);

    t.delete();
    p.reload();
    expect(p.progresso).toBe(100);
    expect(p.status).toBe("Concluído");
  });

  it("mover a tarefa recalcula os dois projetos", () => {
    const origem = criarProjeto();
    const destino = criarProjeto();
    const t = criarTarefa(origem.name);
    t.runMethod("concluir");
    criarTarefa(origem.name);

    origem.reload();
    expect(origem.progresso).toBe(50);

    t.set("projeto", destino.name).save();

    origem.reload();
    destino.reload();
    expect(origem.progresso).toBe(0);
    expect(origem.status).toBe("Em andamento");
    expect(destino.progresso).toBe(100);
    expect(destino.status).toBe("Concluído");
  });
});
