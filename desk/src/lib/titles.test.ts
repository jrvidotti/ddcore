import { describe, it, expect, beforeEach } from "vitest";
import { getLinkTitle, setLinkTitle, registerTitles, clearTitleCache } from "./titles.svelte";
import { formatValue } from "./format";
import { boot } from "./boot.svelte";

describe("titles store", () => {
  beforeEach(() => {
    clearTitleCache();
    boot.data = {
      user: "Admin",
      roles: ["System Manager"],
      userDoc: null,
      lang: "pt-BR",
      langs: [],
      apps: [],
      workspaces: [],
      doctypes: {
        Imovel: { label: "Imóvel", app: "alugueis", icon: "building", module: "Alugueis", titleField: "nome" },
        Pessoa: { label: "Pessoa", app: "alugueis", icon: "user", module: "Alugueis", titleField: "nome_razao_social" },
        SemTitulo: { label: "Sem Título", app: "alugueis", icon: "file", module: "Alugueis" },
      },
      reports: {},
      site: { name: "test", currency: "BRL", timezone: "UTC", dev: true, scheduler: false, version: "0.1.0" },
      loaded: Date.now(),
    };
    boot.ready = true;
  });

  it("registers and retrieves individual title", () => {
    setLinkTitle("Imovel", "IMO-00002", "Loja Centro 12");
    expect(getLinkTitle("Imovel", "IMO-00002")).toBe("Loja Centro 12");
  });

  it("registers titles in batch", () => {
    registerTitles({
      Imovel: { "IMO-00001": "Casa Jardim", "IMO-00002": "Loja Centro 12" },
      Pessoa: { "PES-00001": "João Silva" },
    });
    expect(getLinkTitle("Imovel", "IMO-00001")).toBe("Casa Jardim");
    expect(getLinkTitle("Imovel", "IMO-00002")).toBe("Loja Centro 12");
    expect(getLinkTitle("Pessoa", "PES-00001")).toBe("João Silva");
  });

  it("returns name itself when DocType has no titleField", () => {
    expect(getLinkTitle("SemTitulo", "SEM-001")).toBe("SEM-001");
  });

  it("returns empty string for null or empty values", () => {
    expect(getLinkTitle("Imovel", "")).toBe("");
  });

  it("formatValue formats Link field using title", () => {
    setLinkTitle("Imovel", "IMO-00002", "Loja Centro 12");
    expect(formatValue("IMO-00002", { fieldtype: "Link", options: "Imovel" })).toBe("Loja Centro 12");
  });

  it("formatValue falls back to ID if no title in cache", () => {
    expect(formatValue("IMO-99999", { fieldtype: "Link", options: "Imovel" })).toBe("IMO-99999");
  });
});
