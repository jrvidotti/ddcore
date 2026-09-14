import { describe, it, expect } from "vitest";
import {
  resolveActiveWorkspace,
  resolveWorkspaceForDoctype,
  workspaceItemHref,
  rememberWorkspace,
  getRememberedWorkspace,
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
    expect(workspaceItemHref("Alugueis", { report: "Contratos a Vencer" })).toBe("/app/Alugueis/report/Contratos%20a%20Vencer");
    expect(workspaceItemHref("Alugueis", { route: "/app/Alugueis" })).toBe("/app/Alugueis");
    expect(workspaceItemHref("Alugueis", { label: "Group header" })).toBe("");
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
