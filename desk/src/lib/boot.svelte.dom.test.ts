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
