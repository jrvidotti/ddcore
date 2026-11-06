import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it, vi } from "vitest";
import Chart from "./Chart.svelte";

// jsdom has no ResizeObserver; `bind:clientWidth` only needs it to exist (the chart then draws 800 wide)
globalThis.ResizeObserver ??= class { observe() {} unobserve() {} disconnect() {} } as any;

const days = (n: number) => Array.from({ length: n }, (_, i) => `${String((i % 28) + 1).padStart(2, "0")}/09`);

function render(labels: string[]) {
  const target = document.createElement("div");
  const data = { labels, datasets: [{ name: "A", values: labels.map((_, i) => i) }, { name: "B", values: labels.map(() => 1) }] };
  const view = mount(Chart, { target, props: { data } });
  flushSync();
  // the X labels are the texts at the bottom of the chart (height 220 - 8); the rest are Y ticks
  const xLabels = [...target.querySelectorAll("text")].filter((t) => t.getAttribute("y") === "212").map((t) => t.textContent);
  const bars = target.querySelectorAll("rect").length;
  unmount(view);
  return { xLabels, bars };
}

describe("Chart X labels", () => {
  it("thins a crowded axis and keeps the last label", () => {
    const labels = days(30);
    const { xLabels, bars } = render(labels);
    expect(xLabels).toHaveLength(15);
    expect(xLabels.at(-1)).toBe(labels[29]);
    expect(bars).toBe(60);
  });

  it("draws every label when there is room", () => {
    const labels = days(5);
    expect(render(labels).xLabels).toEqual(labels);
  });
});

function draw(data: any) {
  const target = document.createElement("div");
  const view = mount(Chart, { target, props: { data } });
  flushSync();
  return { target, done: () => unmount(view) };
}

describe("Chart shapes", () => {
  it("draws a donut as slices, not bars", () => {
    const { target, done } = draw({ type: "donut", labels: ["Not due", "1–30", "31–90"], datasets: [{ name: "Balance", values: [70, 20, 10] }] });
    expect(target.querySelectorAll("path")).toHaveLength(3);
    expect(target.querySelectorAll("rect")).toHaveLength(0);
    expect(target.querySelector(".legend")?.textContent).toContain("31–90");
    done();
  });

  it("breaks a line at a null instead of diving to zero", () => {
    const { target, done } = draw({ type: "line", labels: ["Jul", "Aug", "Sep", "Oct"], datasets: [{ name: "INAD90", values: [11.99, 12.1, null, 12.4] }] });
    expect(target.querySelectorAll("polyline")).toHaveLength(1);
    // two drawn points + two hover targets per known point
    expect(target.querySelectorAll("circle")).toHaveLength(6);
    done();
  });

  it("skips the bar of a null", () => {
    const { target, done } = draw({ type: "bar", labels: ["a", "b"], datasets: [{ name: "A", values: [1, null] }] });
    expect(target.querySelectorAll("rect")).toHaveLength(1);
    done();
  });

  it("stacks bars on one column per label", () => {
    const { target, done } = draw({ type: "bar", stacked: true, labels: ["s1", "s2"], datasets: [{ name: "A", values: [1, 2] }, { name: "B", values: [3, 4] }, { name: "C", values: [5, 6] }] });
    const rects = [...target.querySelectorAll("rect")];
    expect(rects).toHaveLength(6);
    expect(new Set(rects.map((r) => r.getAttribute("x"))).size).toBe(2);
    done();
  });

  it("colours a series as it asks and prints the unit on the axis", () => {
    const { target, done } = draw({ type: "line", format: "Percent", labels: ["a", "b"], datasets: [{ name: "Now", values: [10, 12] }, { name: "A year earlier", values: [9, 11], color: "gray" }] });
    const lines = [...target.querySelectorAll("polyline")].map((p) => (p as SVGElement).style.stroke);
    expect(lines).toEqual(["var(--blue)", "var(--gray)"]);
    expect([...target.querySelectorAll("text")].map((t) => t.textContent)).toContain("12%");
    done();
  });

  it("draws its title", () => {
    const { target, done } = draw({ type: "bar", title: "By store", labels: ["a"], datasets: [{ name: "A", values: [1] }] });
    expect(target.querySelector("h3.chart-title")?.textContent).toBe("By store");
    done();
  });
});

describe("Chart drill-down", () => {
  function drawPicking(data: any, pickable?: (i: number) => boolean) {
    const target = document.createElement("div");
    const onpick = vi.fn();
    const view = mount(Chart, { target, props: { data, onpick, pickable } });
    flushSync();
    return { target, onpick, done: () => unmount(view) };
  }

  it("hands over the label of a bar clicked, and only of the labels it may pick", () => {
    const { target, onpick, done } = drawPicking({ type: "bar", labels: ["a", "b", "c"], datasets: [{ name: "A", values: [1, 2, 3] }] }, (i) => i !== 1);
    const rects = [...target.querySelectorAll<SVGElement>("rect")];
    expect(rects.map((r) => r.getAttribute("role"))).toEqual(["button", "img", "button"]);
    rects[2].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    rects[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(onpick.mock.calls).toEqual([[2]]);
    rects[0].dispatchEvent(new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    expect(onpick).toHaveBeenLastCalledWith(0);
    done();
  });

  it("picks a slice and a line point by their label", () => {
    const pie = drawPicking({ type: "pie", labels: ["a", "b"], datasets: [{ name: "A", values: [1, 2] }] });
    pie.target.querySelectorAll("path")[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(pie.onpick).toHaveBeenCalledWith(1);
    pie.done();
    const line = drawPicking({ type: "line", labels: ["a", "b"], datasets: [{ name: "A", values: [1, 2] }] });
    line.target.querySelectorAll("circle[r='10']")[1].dispatchEvent(new MouseEvent("click", { bubbles: true }));
    expect(line.onpick).toHaveBeenCalledWith(1);
    line.done();
  });

  it("takes no click without onpick", () => {
    const { target, done } = draw({ type: "bar", labels: ["a"], datasets: [{ name: "A", values: [1] }] });
    const rect = target.querySelector("rect")!;
    expect(rect.getAttribute("role")).toBe("img");
    expect(rect.getAttribute("tabindex")).toBeNull();
    done();
  });
});
