import { describe, expect, it, vi } from "vitest";

vi.mock("./api", () => ({
  api: {
    boot: async () => ({ user: "ana@x.com", lang: "pt-BR", langs: [] }),
    translations: async () => ({}),
  },
  setRequestLang: () => {},
}));

import { loadBoot } from "./boot.svelte";

describe("loadBoot", () => {
  it("puts the resolved language on <html lang>", async () => {
    document.documentElement.lang = "en";
    await loadBoot();
    expect(document.documentElement.lang).toBe("pt-BR");
  });
});

describe("applyBrand", () => {
  it("writes the site's look into the head, and takes back what a reload dropped (#128)", async () => {
    const { applyBrand } = await import("./boot.svelte");
    applyBrand({ css: ":root:root{--primary:#7c2d2d}", stylesheet: "https://fonts.example/a.css", themeColor: "#7c2d2d", favicon: "/assets/site/favicon?v=1" });
    expect(document.querySelector("style#ddcore-brand")?.textContent).toBe(":root:root{--primary:#7c2d2d}");
    expect(document.querySelector<HTMLLinkElement>("link#ddcore-fonts")?.href).toBe("https://fonts.example/a.css");
    expect(document.querySelector<HTMLMetaElement>('meta[name="theme-color"]')?.content).toBe("#7c2d2d");
    expect(document.querySelector('link[rel="icon"]')?.getAttribute("href")).toBe("/assets/site/favicon?v=1");
    // the same elements are updated, not duplicated
    applyBrand({ css: ":root:root{--primary:#123456}", favicon: "/assets/site/favicon?v=2" });
    expect(document.querySelectorAll("style#ddcore-brand")).toHaveLength(1);
    expect(document.querySelector("style#ddcore-brand")?.textContent).toBe(":root:root{--primary:#123456}");
    expect(document.querySelector("link#ddcore-fonts")).toBeNull();
    expect(document.querySelector('meta[name="theme-color"]')).toBeNull();
    expect(document.querySelectorAll('link[rel="icon"]')).toHaveLength(1);
    applyBrand(undefined);
    expect(document.querySelector("style#ddcore-brand")).toBeNull();
  });
});
