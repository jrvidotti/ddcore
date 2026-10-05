import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

const reply = vi.hoisted(() => ({ meta: {} as any }));

vi.mock("$lib/boot.svelte", () => ({
  boot: { data: { reports: {} } },
  __: (s: string) => s,
}));
vi.mock("$lib/api", () => ({
  api: { report: vi.fn(async () => ({ meta: reply.meta, result: { columns: [], rows: [] } })) },
}));
vi.mock("$lib/routes", () => ({ seg: (s: string) => s }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));
vi.mock("$app/state", () => ({ page: { url: new URL("http://localhost/app/report/Balances"), state: {} } }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$lib/desk-sdk", () => ({ deskSDK: {} }));
vi.mock("./sidebar-workspace", () => ({ getRememberedWorkspace: () => "" }));
vi.mock("./ReportGrid.svelte", () => ({ default: () => {} }));
vi.mock("./BarChart.svelte", () => ({ default: () => {} }));
vi.mock("$lib/controls/Control.svelte", () => ({ default: () => {} }));

import { api } from "$lib/api";
import ReportView from "./ReportView.svelte";

async function render(meta: any) {
  reply.meta = meta;
  const target = document.createElement("div");
  const view = mount(ReportView, { target, props: { name: "Balances" } });
  await vi.waitFor(() => expect(api.report).toHaveBeenCalled());
  await new Promise((r) => setTimeout(r, 0)); // let the reply land in the component
  flushSync();
  return { target, view };
}

describe("ReportView", () => {
  it("draws the report's description under its title", async () => {
    const { target, view } = await render({ label: "Balances", description: "What each account holds today", filters: [] });
    const title = target.querySelector(".page-head .page-title");
    expect(title?.querySelector("h1")?.textContent).toBe("Balances");
    expect(title?.querySelector("p.subtitle")?.textContent).toBe("What each account holds today");
    unmount(view);
  });
  it("draws no subtitle when the report has no description", async () => {
    const { target, view } = await render({ label: "Balances", filters: [] });
    expect(target.querySelector("p.subtitle")).toBeNull();
    unmount(view);
  });
});
