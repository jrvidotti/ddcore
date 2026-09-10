import "@ddcore/sdk/test";
import { generate } from "./demo";

// O banco de teste pode já ter rodado `ddcore demo`; cada `it` roda em transação
// revertida, então limpar aqui não afeta os outros testes nem o banco.
function limparDemo() {
  for (const t of ddcore.db.getAll<{ name: string }>("Tarefa", { filters: { projeto: "DEMO" }, fields: ["name"] })) {
    ddcore.deleteDoc("Tarefa", t.name, { force: true });
  }
  if (ddcore.db.exists("Projeto", "DEMO")) ddcore.deleteDoc("Projeto", "DEMO", { force: true });
}

describe("demo", () => {
  beforeEach(limparDemo);

  it("cria o projeto DEMO com marcos e tarefas", () => {
    const r = generate();
    expect(r.criados).toContain("DEMO");
    expect(r.criados).toContain("DEMO-01");
    expect(r.criados).toContain("DEMO-02");
    expect(r.criados).toContain("DEMO-03");
    expect(r.quantidade).toBe(4);

    expect(ddcore.db.count("Marco Projeto", { parent: "DEMO" })).toBe(3);
    expect(ddcore.db.getValue("Tarefa", "DEMO-01", "status")).toBe("Aberta");
    expect(ddcore.db.getValue("Tarefa", "DEMO-02", "status")).toBe("Em andamento");
    expect(ddcore.db.getValue("Tarefa", "DEMO-03", "status")).toBe("Concluída");
    expect(ddcore.db.getValue("Projeto", "DEMO", "progresso")).toBeCloseTo(33.33, 2);
  });

  it("a segunda execução não duplica nada", () => {
    generate();
    const r = generate();
    expect(r.quantidade).toBe(0);
    expect(r.criados).toHaveLength(0);
    expect(ddcore.db.count("Projeto", { codigo: "DEMO" })).toBe(1);
    expect(ddcore.db.count("Tarefa", { projeto: "DEMO" })).toBe(3);
    expect(ddcore.db.count("Marco Projeto", { parent: "DEMO" })).toBe(3);
  });
});
