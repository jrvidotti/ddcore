import { describe, it, expect } from "vitest";
import {
  resolveActiveWorkspace,
  resolveWorkspaceForDoctype,
  workspaceItemHref,
  workspaceRedirect,
  rememberWorkspace,
  getRememberedWorkspace,
  landingWorkspace,
  type WorkspaceItem,
} from "./sidebar-workspace";

const mockWorkspaces: WorkspaceItem[] = [
  {
    name: "Alugueis",
    label: "Aluguéis",
    icon: "building-2",
    app: "alugueis",
    sidebar: [
      { label: "Visão Geral", route: "/app/Alugueis", icon: "layout-dashboard" },
      { label: "Contratos", doctype: "Contrato", icon: "notepad-text" },
      { label: "Imóveis", doctype: "Imovel", icon: "building-2" },
      { label: "Relatórios", icon: "bar-chart-3" },
      { label: "Contratos a Vencer", report: "Contratos a Vencer" },
    ],
  },
  {
    name: "Manutencao",
    label: "Maintenance",
    icon: "settings",
    app: "manutencao",
    sidebar: [
      { label: "Overview", route: "/app/Manutencao", icon: "layout-dashboard" },
      { label: "Ordens de Serviço", doctype: "OrdemServico", icon: "wrench" },
    ],
  },
];

const mockDoctypes = {
  Contrato: { label: "Contrato", app: "alugueis", icon: "file", module: "Alugueis" },
  Imovel: { label: "Imóvel", app: "alugueis", icon: "building", module: "Alugueis" },
  OrdemServico: { label: "Ordem de Serviço", app: "manutencao", icon: "wrench", module: "Manutencao" },
  User: { label: "User", app: "core", icon: "user", module: "Core" },
};

describe("sidebar-workspace", () => {
  it("resolves active workspace from direct workspace route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Manutencao",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("resolves active workspace from prefixed doctype route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Alugueis/Contrato",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("resolves active workspace from prefixed document form route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/Alugueis/Contrato/CTR-0001",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("resolves active workspace for legacy un-prefixed doctype route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/OrdemServico",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("resolves active workspace for legacy report route", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/report/Contratos%20a%20Vencer",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("falls back to remembered workspace on global routes like /app/notifications", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/notifications",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
      remembered: "Manutencao",
    });
    expect(ws?.name).toBe("Manutencao");
  });

  it("falls back to first workspace if nothing is remembered", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/unknown",
      workspaces: mockWorkspaces,
      doctypes: mockDoctypes,
    });
    expect(ws?.name).toBe("Alugueis");
  });

  it("falls back to the site's home on bare /app when the remembered workspace is the other space's (#127)", () => {
    const at = (remembered?: string, home?: string) =>
      resolveActiveWorkspace({ currentPath: "/app", workspaces: mockWorkspaces, doctypes: mockDoctypes, remembered, home })?.name;
    expect(at("Platform", "Manutencao")).toBe("Manutencao");
    expect(at("", "manutencao")).toBe("Manutencao");
    expect(at("Alugueis", "Manutencao")).toBe("Alugueis");
    expect(at("Platform", "Platform")).toBe("Alugueis");
  });

  it("returns null when workspaces list is empty", () => {
    const ws = resolveActiveWorkspace({
      currentPath: "/app/notifications",
      workspaces: [],
      doctypes: mockDoctypes,
    });
    expect(ws).toBeNull();
  });

  it("resolves owning workspace for a doctype", () => {
    expect(resolveWorkspaceForDoctype("Contrato", mockWorkspaces, mockDoctypes)).toBe("Alugueis");
    expect(resolveWorkspaceForDoctype("OrdemServico", mockWorkspaces, mockDoctypes)).toBe("Manutencao");
    expect(resolveWorkspaceForDoctype("NonExistent", mockWorkspaces, mockDoctypes)).toBeNull();
  });

  it("generates correct workspace prefixed hrefs", () => {
    expect(workspaceItemHref("Alugueis", { doctype: "Contrato" })).toBe("/app/Alugueis/Contrato");
    expect(workspaceItemHref("Alugueis", { report: "Contratos a Vencer" })).toBe("/app/Alugueis/report/ContratosaVencer");
    expect(workspaceItemHref("Alugueis", { route: "/app/Alugueis" })).toBe("/app/Alugueis");
    expect(workspaceItemHref("Alugueis", { label: "Group header" })).toBe("");
  });

  it("keeps the query string and the hash of a short route through the workspace redirect", () => {
    expect(workspaceRedirect("Training", "Training Class", ["new"], { search: "?course=C-001", hash: "" }))
      .toBe("/app/Training/TrainingClass/new?course=C-001");
    expect(workspaceRedirect("Training", "Course", [], { search: "?is_active=1", hash: "#top" }))
      .toBe("/app/Training/Course?is_active=1#top");
    expect(workspaceRedirect("Training", "Course", ["C 001"], { search: "", hash: "#notes" }))
      .toBe("/app/Training/Course/C%20001#notes");
    expect(workspaceRedirect("Human Resources", "Course", [], { search: "", hash: "" }))
      .toBe("/app/HumanResources/Course");
    // no workspace owns the DocType: it stays on the short route, without its spaces
    expect(workspaceRedirect(null, "Audit Event", ["AE 1"], { search: "?tab=Log", hash: "" }))
      .toBe("/app/AuditEvent/AE%201?tab=Log");
  });

  it("finds the active workspace from a path under route names or under the names themselves", () => {
    const workspaces: WorkspaceItem[] = [
      { name: "Projects", label: "Projects", sidebar: [{ label: "Items", doctype: "Work Item" }] },
      { name: "Human Resources", label: "HR", sidebar: [{ label: "Classes", doctype: "Training Class" }, { label: "Open", report: "Open Classes" }] },
    ];
    const at = (currentPath: string) => resolveActiveWorkspace({ currentPath, workspaces, doctypes: { "Work Item": {}, "Training Class": {} } })?.name;
    expect(at("/app/HumanResources/TrainingClass")).toBe("Human Resources");
    expect(at("/app/Human%20Resources")).toBe("Human Resources");
    expect(at("/app/TrainingClass/new")).toBe("Human Resources");
    expect(at("/app/Training%20Class")).toBe("Human Resources");
    expect(at("/app/report/OpenClasses")).toBe("Human Resources");
    expect(at("/app/workspace/HumanResources")).toBe("Human Resources");
  });

  it("remembers and retrieves workspace in storage safely", () => {
    const memory = new Map<string, string>();
    const store = {
      getItem: (k: string) => memory.get(k) ?? null,
      setItem: (k: string, v: string) => { memory.set(k, v); },
    };
    rememberWorkspace("Manutencao", store);
    expect(getRememberedWorkspace(store)).toBe("Manutencao");
  });
});

describe("landingWorkspace", () => {
  const ws = [{ name: "Back Office" }, { name: "Sales" }];
  it("opens the remembered workspace, then the home, then the first", () => {
    expect(landingWorkspace(ws, "sales", "Back Office")).toBe("Sales");
    expect(landingWorkspace(ws, "", "Sales")).toBe("Sales");
    expect(landingWorkspace(ws, "", undefined)).toBe("Back Office");
  });
  it("skips a remembered workspace or a home boot did not bring (the other space's)", () => {
    expect(landingWorkspace(ws, "Tenants", "Tenants")).toBe("Back Office");
    expect(landingWorkspace([], "Sales", "Sales")).toBe("");
  });
});
