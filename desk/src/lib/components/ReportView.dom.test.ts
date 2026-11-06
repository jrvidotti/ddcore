import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";

const reply = vi.hoisted(() => ({ meta: {} as any, result: { columns: [], rows: [] } as any }));

vi.mock("$lib/boot.svelte", () => ({
  boot: { data: { reports: {} } },
  __: (s: string) => s,
}));
vi.mock("$lib/api", () => ({
  api: { report: vi.fn(async () => ({ meta: reply.meta, result: reply.result })) },
}));
vi.mock("$lib/routes", () => ({ seg: (s: string) => s, appPrefix: () => "/app" }));
vi.mock("$lib/ui.svelte", () => ({ showError: vi.fn() }));
const page = vi.hoisted(() => ({ url: new URL("http://localhost/app/report/Balances"), state: {}, params: {} as Record<string, string> }));
vi.mock("$app/state", () => ({ page }));
vi.mock("$app/navigation", () => ({ goto: vi.fn() }));
vi.mock("$lib/desk-sdk", () => ({ deskSDK: { ddcore: { datetime: {} } } }));
vi.mock("./sidebar-workspace", () => ({ getRememberedWorkspace: () => "" }));
const handed = vi.hoisted(() => ({ grid: null as any, charts: [] as any[] }));
vi.mock("./ReportGrid.svelte", () => ({ default: (_: any, props: any) => { handed.grid = props; } }));
vi.mock("./Chart.svelte", () => ({ default: (_: any, props: any) => { handed.charts.push(props); } }));
vi.mock("$lib/controls/Control.svelte", () => ({ default: () => {} }));

import { api } from "$lib/api";
import { goto } from "$app/navigation";
import ReportView from "./ReportView.svelte";

async function render(meta: any, result: any = { columns: [], rows: [] }) {
  reply.meta = meta;
  reply.result = result;
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
  it("draws `chart` and every one of `charts`, side by side", async () => {
    const bar = { type: "bar", labels: ["a"], datasets: [{ name: "A", values: [1] }] };
    const { target, view } = await render({ label: "Balances", filters: [] }, { columns: [], rows: [], chart: bar, charts: [{ ...bar, type: "line" }] });
    expect(target.querySelectorAll(".chart-card")).toHaveLength(2);
    expect(target.querySelector(".charts")?.classList.contains("pair")).toBe(true);
    unmount(view);
  });
  it("draws no chart when the report has none", async () => {
    const { target, view } = await render({ label: "Balances", filters: [] });
    expect(target.querySelector(".charts")).toBeNull();
    unmount(view);
  });
});

describe("ReportView drill-down", () => {
  const filters = [
    { fieldname: "group_by", fieldtype: "Select", options: "Risk Band\nAge Range", default: "Risk Band" },
    { fieldname: "risk_band", fieldtype: "Select", options: "\n1\n6" },
    { fieldname: "store", fieldtype: "Data" },
  ];
  const columns = [{ fieldname: "group", label: "Group", fieldtype: "Data" }];

  it("reopens the report with a row's drill over the current filters, through the URL", async () => {
    page.url = new URL("http://localhost/app/report/Rates?store=Centro");
    vi.mocked(goto).mockClear();
    const rows = [{ group: "Band 6", drill: { risk_band: "6", group_by: "Age Range" } }, { group: "Other" }];
    const { view } = await render({ label: "Rates", filters }, { columns, rows, totals: { group: "All" } });
    expect(handed.grid.reportTotals).toEqual({ group: "All" });
    expect(handed.grid.rowclickable(rows[0])).toBe(true);
    expect(handed.grid.rowclickable(rows[1])).toBe(false);
    handed.grid.onrowclick(rows[0]);
    expect(goto).toHaveBeenCalledWith("/app/report/Balances?group_by=Age+Range&risk_band=6&store=Centro", { noScroll: true });
    unmount(view);
    page.url = new URL("http://localhost/app/report/Balances");
  });

  it("drills from a chart label, and gives a grid without drills no row click", async () => {
    vi.mocked(goto).mockClear();
    handed.charts = [];
    const chart = { type: "bar", labels: ["Band 1", "Band 6"], datasets: [{ name: "Rate", values: [3, 28] }], drill: [null, { risk_band: "6" }] };
    const { view } = await render({ label: "Rates", filters }, { columns, rows: [{ group: "Band 1" }], chart });
    expect(handed.grid.onrowclick).toBeUndefined();
    const c = handed.charts.at(-1);
    expect([c.pickable(0), c.pickable(1)]).toEqual([false, true]);
    c.onpick(1);
    expect(goto).toHaveBeenCalledWith("/app/report/Balances?group_by=Risk+Band&risk_band=6", { noScroll: true });
    unmount(view);
  });

  it("gives a chart without drills no click", async () => {
    handed.charts = [];
    const bar = { type: "bar", labels: ["a"], datasets: [{ name: "A", values: [1] }] };
    const { view } = await render({ label: "Rates", filters }, { columns, rows: [], chart: bar });
    expect(handed.charts.at(-1).onpick).toBeUndefined();
    unmount(view);
  });
});
