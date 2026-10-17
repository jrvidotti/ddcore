import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

const values: Record<string, any> = {
  short: { value: 481506.93, formatted: "R$ 481.506,93" },
  long: { value: 226, formatted: "226 granted · 885 denied · 3170 pending" },
};

vi.mock("$lib/boot.svelte", () => ({
  boot: { data: { workspaces: [{ name: "Portal", numberCards: [{ name: "short", label: "Short" }, { name: "long", label: "Long" }] }] } },
  __: (s: string) => s,
}));
vi.mock("$lib/api", () => ({
  api: { numberCard: vi.fn(async (_ws: string, card: string) => values[card]), chart: vi.fn() },
}));
vi.mock("$lib/routes", () => ({ seg: (s: string) => s, resolveWorkspace: (s: string) => s }));
vi.mock("./BarChart.svelte", () => ({ default: () => {} }));
vi.mock("./Icon.svelte", () => ({ default: () => {} }));

import Workspace from "./Workspace.svelte";

describe("Workspace number cards", () => {
  it("shrinks a value that reads as a sentence, and only that one", async () => {
    const target = document.createElement("div");
    const view = mount(Workspace, { target, props: { name: "Portal" } });
    await vi.waitFor(() => {
      flushSync();
      expect(target.textContent).toContain("3170 pending");
    });
    const [short, long] = [...target.querySelectorAll(".ncard .v")];
    expect(short.classList.contains("long")).toBe(false);
    expect(long.classList.contains("long")).toBe(true);
    unmount(view);
  });
});
