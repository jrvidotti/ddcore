import { describe, expect, it } from "vitest";
import { arcPath, decimalsOf, domain, formatPoint, formatTick, lineSegments, num, pieSlices, scale, seriesColors, stackBars } from "./chart-layout";

describe("chart-layout", () => {
  it("reads null, undefined, empty and non-numbers as unknown, not as 0", () => {
    expect([null, undefined, "", "x", 0, "1.5"].map(num)).toEqual([null, null, null, null, 0, 1.5]);
  });

  it("gives a declared colour to its series and the free ones of the cycle to the rest", () => {
    expect(seriesColors([undefined, "gray", undefined])).toEqual(["var(--blue)", "var(--gray)", "var(--red)"]);
    // a declared blue is not handed out again
    expect(seriesColors(["blue", undefined])).toEqual(["var(--blue)", "var(--red)"]);
    // an unknown name falls back to the cycle
    expect(seriesColors(["pink"])).toEqual(["var(--blue)"]);
    // six series, six colours
    expect(new Set(seriesColors(Array(6).fill(undefined))).size).toBe(6);
  });

  it("leaves unknown points out of the domain", () => {
    expect(domain([{ name: "a", values: [11.99, null, null] }, { name: "b", values: [10.28, 10.64, 10.63] }], 3)).toEqual({ min: 0, max: 11.99 });
  });

  it("lets a line leave zero out of its domain", () => {
    expect(domain([{ name: "a", values: [11.99, 10.28, null] }], 3, false, false)).toEqual({ min: 10.28, max: 11.99 });
    expect(domain([{ name: "a", values: [null] }], 1, false, false)).toEqual({ min: 0, max: 0 });
  });

  it("measures stacked bars by their sums, positives up and negatives down", () => {
    const ds = [{ name: "a", values: [5, -2] }, { name: "b", values: [3, -4] }, { name: "c", values: [null, 1] }];
    expect(domain(ds, 2, true)).toEqual({ min: -6, max: 8 });
    expect(stackBars(ds, 2)).toEqual([[[0, 5], [-2, 0]], [[5, 8], [-6, -2]], [null, [0, 1]]]);
  });

  it("puts the axis on round ticks that cover the domain", () => {
    expect(scale(0, 13.4)).toEqual({ min: 0, max: 15, ticks: [0, 5, 10, 15], step: 5 });
    expect(scale(-6, 8).ticks).toEqual([-10, -5, 0, 5, 10]);
    expect(scale(0, 0).ticks).toEqual([0, 0.25, 0.5, 0.75, 1]);
    expect(decimalsOf(0.25)).toBe(2);
    expect(decimalsOf(5)).toBe(0);
  });

  it("breaks a line at an unknown point", () => {
    expect(lineSegments([1, 2, null, 4, null, null, 7, 8], 8)).toEqual([[0, 1], [3], [6, 7]]);
    expect(lineSegments([null, null], 2)).toEqual([]);
  });

  it("cuts a pie from twelve o'clock, skipping unknown and empty slices", () => {
    const s = pieSlices([1, null, 0, 3]);
    expect(s.map((x) => [x.index, x.share])).toEqual([[0, 0.25], [3, 0.75]]);
    expect(s[0].start).toBeCloseTo(-Math.PI / 2);
    expect(s[1].end).toBeCloseTo((3 * Math.PI) / 2);
    expect(pieSlices([null, 0])).toEqual([]);
  });

  it("draws a whole circle as two halves, which an arc can do", () => {
    const d = arcPath(100, 100, 50, 30, -Math.PI / 2, (3 * Math.PI) / 2);
    expect(d.match(/M/g)).toHaveLength(2);
  });

  it("prints a unit on the axis and the tooltip", () => {
    expect(formatTick(12, "Percent", 2)).toBe("12%");
    expect(formatTick(0.5, "Percent", 0.25)).toBe("0.50%");
    expect(formatTick(7582543.84, "Currency", 2000000)).toBe("$7.6M");
    expect(formatTick(12, undefined, 2)).toBe("12");
    expect(formatPoint(11.99, "Percent")).toBe("11.99%");
    expect(formatPoint(7582543.84, "Currency")).toBe("$7,582,543.84");
    expect(formatPoint(1.5, undefined)).toBe("1.50");
  });
});
