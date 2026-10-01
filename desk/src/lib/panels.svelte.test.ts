import { describe, expect, it } from "vitest";
import {
  DOC_SIDEBAR_KEY,
  SIDEBAR_KEY,
  panelCollapsed,
  panels,
  rememberPanelCollapsed,
  setDocSidebarCollapsed,
  setSidebarCollapsed,
} from "./panels.svelte";

function memoryStore(initial: Record<string, string> = {}) {
  const data = new Map(Object.entries(initial));
  return {
    data,
    getItem: (k: string) => data.get(k) ?? null,
    setItem: (k: string, v: string) => void data.set(k, v),
  };
}

const brokenStore = {
  getItem: () => { throw new Error("denied"); },
  setItem: () => { throw new Error("denied"); },
};

describe("collapsed side bars", () => {
  it("starts expanded when nothing is stored, or what is stored is not a choice", () => {
    expect(panelCollapsed(SIDEBAR_KEY, memoryStore())).toBe(false);
    expect(panelCollapsed(SIDEBAR_KEY, memoryStore({ [SIDEBAR_KEY]: "yes" }))).toBe(false);
    expect(panelCollapsed(SIDEBAR_KEY, null)).toBe(false);
  });

  it("remembers each bar's choice on its own", () => {
    const store = memoryStore();
    rememberPanelCollapsed(SIDEBAR_KEY, true, store);
    expect(panelCollapsed(SIDEBAR_KEY, store)).toBe(true);
    expect(panelCollapsed(DOC_SIDEBAR_KEY, store)).toBe(false);
    rememberPanelCollapsed(SIDEBAR_KEY, false, store);
    expect(panelCollapsed(SIDEBAR_KEY, store)).toBe(false);
  });

  it("survives a storage that throws", () => {
    expect(panelCollapsed(SIDEBAR_KEY, brokenStore)).toBe(false);
    expect(() => rememberPanelCollapsed(SIDEBAR_KEY, true, brokenStore)).not.toThrow();
  });

  it("updates the shared state and the storage together", () => {
    const store = memoryStore();
    setSidebarCollapsed(true, store);
    setDocSidebarCollapsed(true, store);
    expect(panels).toEqual({ sidebarCollapsed: true, docSidebarCollapsed: true });
    expect(store.data.get(SIDEBAR_KEY)).toBe("collapsed");
    expect(store.data.get(DOC_SIDEBAR_KEY)).toBe("collapsed");
    setSidebarCollapsed(false, store);
    setDocSidebarCollapsed(false, store);
    expect(panels).toEqual({ sidebarCollapsed: false, docSidebarCollapsed: false });
    expect(store.data.get(SIDEBAR_KEY)).toBe("expanded");
  });
});
