import "@cerne/sdk/test";
import { gerar } from "./demo";

// O banco de teste pode já ter rodado `cerne demo`; cada `it` roda em transação
// revertida, então limpar aqui não afeta os outros testes nem o banco.
function limparDemo() {
  for (const t of cerne.db.getAll<{ name: string }>("Tarefa", { filters: { projeto: "DEMO" }, fields: ["name"] })) {
    cerne.deleteDoc("Tarefa", t.name, { force: true });
  }
  if (cerne.db.exists("Projeto", "DEMO")) cerne.deleteDoc("Projeto", "DEMO", { force: true });
}

describe("demo", () => {
  beforeEach(limparDemo);

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
