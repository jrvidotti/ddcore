import { describe, expect, it } from "vitest";
import { resolveActiveTab, tabToSearchParams } from "./form-tabs";

describe("form tabs URL state", () => {
  const tabs = [{ label: "Details" }, { label: "Financeiro" }, { label: "Reajuste" }, { label: "Documentos" }];

  it("resolves active tab case-insensitively", () => {
    expect(resolveActiveTab(tabs, "financeiro")).toBe(1);
    expect(resolveActiveTab(tabs, "Financeiro")).toBe(1);
    expect(resolveActiveTab(tabs, "REAJUSTE")).toBe(2);
    expect(resolveActiveTab(tabs, "Documentos")).toBe(3);
  });

  it("defaults to 0 on unknown tab or empty param", () => {
    expect(resolveActiveTab(tabs, "")).toBe(0);
    expect(resolveActiveTab(tabs, null)).toBe(0);
    expect(resolveActiveTab(tabs, "inexistente")).toBe(0);
  });

  it("updates search params deleting tab on first tab and preserving other params", () => {
    const existing = new URLSearchParams("edit=1&imovel=IMO-1");
    const p1 = tabToSearchParams(tabs, 1, existing);
    expect(p1.get("tab")).toBe("Financeiro");
    expect(p1.get("edit")).toBe("1");
    expect(p1.get("imovel")).toBe("IMO-1");

    const p0 = tabToSearchParams(tabs, 0, p1);
    expect(p0.has("tab")).toBe(false);
    expect(p0.get("edit")).toBe("1");
  });
});
