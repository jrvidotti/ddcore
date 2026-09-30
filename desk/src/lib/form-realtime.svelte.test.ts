import { describe, expect, it, vi } from "vitest";
import { FormController } from "./form.svelte";
import type { Meta } from "./meta";

const subs = new Map<string, Set<(p: any) => void>>();
vi.mock("./events", () => ({
  subscribe: (event: string, h: (p: any) => void) => {
    if (!subs.has(event)) subs.set(event, new Set());
    subs.get(event)!.add(h);
    return () => subs.get(event)!.delete(h);
  },
}));
vi.mock("./api", () => ({ api: {} }));
vi.mock("./ui.svelte", () => ({ toast: vi.fn(), showError: vi.fn(), ui: { busy: 0 } }));
vi.mock("./boot.svelte", () => ({ __: (s: string) => s }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));

const meta = {
  doctype: { name: "Chat Session", label: "Chat Session", formApps: [], fields: [] },
  permissions: { read: true, write: true },
} as unknown as Meta;

const emit = (event: string, p: any) => subs.get(event)?.forEach((h) => h(p));

describe("frm.onRealtime", () => {
  it("delivers events until the form is disposed", () => {
    const f = new FormController(meta, { doctype: "Chat Session", id: "CS-1" });
    const got: any[] = [];
    f.onRealtime("my_app.chat", (p) => got.push(p));
    emit("my_app.chat", { session: "CS-1" });
    f.dispose();
    emit("my_app.chat", { session: "CS-1" });
    expect(got).toEqual([{ session: "CS-1" }]);
    expect(subs.get("my_app.chat")!.size).toBe(0);
  });
});
