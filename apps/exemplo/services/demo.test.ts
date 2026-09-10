import "@cerne/sdk/test";
import { gerar } from "./demo";

describe("demo", () => {
  it("cria o projeto DEMO com marcos e tarefas", () => {
    const r = gerar();
    expect(r.criados).toContain("DEMO");
    expect(r.criados).toContain("DEMO-01");
    expect(r.criados).toContain("DEMO-02");
    expect(r.criados).toContain("DEMO-03");
    expect(r.quantidade).toBe(4);

    expect(cerne.db.count("Marco Projeto", { parent: "DEMO" })).toBe(3);
    expect(cerne.db.getValue("Tarefa", "DEMO-01", "status")).toBe("Aberta");
    expect(cerne.db.getValue("Tarefa", "DEMO-02", "status")).toBe("Em andamento");
    expect(cerne.db.getValue("Tarefa", "DEMO-03", "status")).toBe("Concluída");
    expect(cerne.db.getValue("Projeto", "DEMO", "progresso")).toBeCloseTo(33.33, 2);
  });

  it("a segunda execução não duplica nada", () => {
    gerar();
    const r = gerar();
    expect(r.quantidade).toBe(0);
    expect(r.criados).toHaveLength(0);
    expect(cerne.db.count("Projeto", { codigo: "DEMO" })).toBe(1);
    expect(cerne.db.count("Tarefa", { projeto: "DEMO" })).toBe(3);
    expect(cerne.db.count("Marco Projeto", { parent: "DEMO" })).toBe(3);
  });
});
