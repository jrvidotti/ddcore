import { flushSync, mount, unmount } from "svelte";
import { describe, expect, it } from "vitest";
import BarChart from "./BarChart.svelte";

const days = (n: number) => Array.from({ length: n }, (_, i) => `${String((i % 28) + 1).padStart(2, "0")}/09`);

function render(labels: string[]) {
  const target = document.createElement("div");
  const data = { labels, datasets: [{ name: "A", values: labels.map((_, i) => i) }, { name: "B", values: labels.map(() => 1) }] };
  const view = mount(BarChart, { target, props: { data } });
  flushSync();
  // the X labels are the texts at the bottom of the chart (height 220 - 8); the rest are Y ticks
  const xLabels = [...target.querySelectorAll("text")].filter((t) => t.getAttribute("y") === "212").map((t) => t.textContent);
  const bars = target.querySelectorAll("rect").length;
  unmount(view);
  return { xLabels, bars };
}

describe("BarChart X labels", () => {
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
