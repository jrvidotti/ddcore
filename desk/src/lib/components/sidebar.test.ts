import { describe, it, expect } from "vitest";
import { isActiveLink, systemDoctypes } from "./sidebar";
import type { Boot } from "$lib/boot.svelte";

function bootWith(roles: string[]): Boot {
  return {
    user: "ana@x.com",
    roles,
    userDoc: null,
    lang: "pt-BR",
    langs: [],
    apps: [],
    workspaces: [],
    doctypes: {
      User: { label: "Usuário", app: "core", icon: "user", module: "Core" },
      Role: { label: "Papel", app: "core", icon: "shield", module: "Core" },
      Imovel: { label: "Imóvel", app: "alugueis", icon: "building", module: "Alugueis" },
    },
    reports: {},
    site: { name: "test", currency: "BRL", timezone: "UTC", dev: true, scheduler: false, version: "0.1.0" },
    loaded: Date.now(),
  };
}

describe("systemDoctypes", () => {
  it("lists the core doctypes for a System Manager, sorted", () => {
    expect(systemDoctypes(bootWith(["All", "System Manager"])).map(([n]) => n)).toEqual(["Role", "User"]);
  });

  it("gives an ordinary user nothing — the group is an administration shortcut", () => {
    expect(systemDoctypes(bootWith(["All", "Gestor"]))).toEqual([]);
  });

  it("leaves an app's own doctypes to its workspace", () => {
    expect(systemDoctypes(bootWith(["System Manager"])).map(([n]) => n)).not.toContain("Imovel");
  });

  it("survives being asked before the boot payload arrived", () => {
    expect(systemDoctypes(null)).toEqual([]);
  });
});

describe("isActiveLink", () => {
  const ws = ["Alugueis", "Manutencao"];

  it("lights a DocType link on its list and its forms", () => {
    expect(isActiveLink("/app/Alugueis/Contrato", "/app/Alugueis/Contrato", ws)).toBe(true);
    expect(isActiveLink("/app/Alugueis/Contrato/CTR-1", "/app/Alugueis/Contrato", ws)).toBe(true);
    expect(isActiveLink("/app/Alugueis/Imovel", "/app/Alugueis/Contrato", ws)).toBe(false);
  });

  it("lights the workspace dashboard only on the dashboard", () => {
    expect(isActiveLink("/app/Alugueis", "/app/Alugueis", ws)).toBe(true);
    expect(isActiveLink("/app/alugueis/", "/app/Alugueis", ws)).toBe(true);
    expect(isActiveLink("/app/Alugueis/Contrato", "/app/Alugueis", ws)).toBe(false);
  });

  it("treats the legacy dashboard route the same way", () => {
    expect(isActiveLink("/app/Alugueis", "/app/workspace/Alugueis", ws)).toBe(true);
    expect(isActiveLink("/app/Alugueis/Contrato", "/app/workspace/Alugueis", ws)).toBe(false);
  });

  it("matches encoded names", () => {
    expect(isActiveLink("/app/Alugueis/Configuracao%20Alugueis", "/app/Alugueis/Configuracao%20Alugueis", ws)).toBe(true);
  });
});
