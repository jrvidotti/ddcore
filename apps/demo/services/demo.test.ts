import "@ddcore/sdk/test";
import { generate } from "./demo";

// The test database may already have run `ddcore demo`; each `it` runs in a
// rolled-back transaction, so cleaning up here affects no other test and no row.
function limparDemo() {
  for (const t of ddcore.db.getAll<{ name: string }>("Task", { filters: { project: "DEMO" }, fields: ["name"] })) {
    ddcore.deleteDoc("Task", t.name, { force: true });
  }
  if (ddcore.db.exists("Project", "DEMO")) ddcore.deleteDoc("Project", "DEMO", { force: true });
}

describe("demo", () => {
  beforeEach(limparDemo);

  it("cria o project DEMO com milestones e tasks", () => {
    const r = generate();
    expect(r.created).toContain("DEMO");
    expect(r.created).toContain("DEMO-01");
    expect(r.created).toContain("DEMO-02");
    expect(r.created).toContain("DEMO-03");
    expect(r.count).toBe(4);

    expect(ddcore.db.count("Project Milestone", { parent: "DEMO" })).toBe(3);
    expect(ddcore.db.getValue("Task", "DEMO-01", "status")).toBe("Open");
    expect(ddcore.db.getValue("Task", "DEMO-02", "status")).toBe("In progress");
    expect(ddcore.db.getValue("Task", "DEMO-03", "status")).toBe("Completed");
    expect(ddcore.db.getValue("Project", "DEMO", "progress")).toBeCloseTo(33.33, 2);
  });

  it("a second run duplicates nothing", () => {
    generate();
    const r = generate();
    expect(r.count).toBe(0);
    expect(r.created).toHaveLength(0);
    expect(ddcore.db.count("Project", { code: "DEMO" })).toBe(1);
    expect(ddcore.db.count("Task", { project: "DEMO" })).toBe(3);
    expect(ddcore.db.count("Project Milestone", { parent: "DEMO" })).toBe(3);
  });
});
