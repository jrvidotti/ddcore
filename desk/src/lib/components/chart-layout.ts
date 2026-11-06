// The arithmetic behind Chart.svelte, kept apart so it is tested without a DOM:
// which colour each series takes, the value scale, where a line breaks, how bars
// stack, the slices of a pie and how a value is printed on the axis and the tooltip.
import { formatNumber, formatValue } from "$lib/format";
import { currencyFmt, numberFmt } from "$lib/locale";

export type ChartFormat = "Percent" | "Currency" | "Float" | "Int";

export interface ChartDataset {
  name: string;
  values: (number | null)[];
  color?: string;
  format?: string;
}

export interface ChartData {
  type?: string;
  title?: string;
  labels: string[];
  datasets: ChartDataset[];
  stacked?: boolean;
  format?: string;
}

/** The indicator colours a series may name; each is a token in app.css. */
export const PALETTE = ["blue", "green", "orange", "red", "purple", "gray", "yellow"];
// The colours handed out to series that name none. Gray is left out: it is
// what a reference series ("a year earlier") asks for, and it should stand apart.
const CYCLE = ["blue", "red", "green", "orange", "purple", "yellow"];

/** A point's value, or null when it is not known (null, undefined, "" or not a number). */
export function num(v: unknown): number | null {
  if (v === null || v === undefined || v === "") return null;
  const n = Number(v);
  return Number.isFinite(n) ? n : null;
}

/**
 * The CSS colour of each series: the one it names, or the next one of the
 * cycle that no series named, so an automatic colour never repeats a declared one.
 */
export function seriesColors(declared: (string | undefined)[]): string[] {
  const valid = (c?: string): c is string => !!c && PALETTE.includes(c);
  const taken = new Set(declared.filter(valid));
  const free = CYCLE.filter((c) => !taken.has(c));
  const pool = free.length ? free : CYCLE;
  let k = 0;
  return declared.map((c) => `var(--${valid(c) ? c : pool[k++ % pool.length]})`);
}

/**
 * The lowest and highest value drawn; stacked bars count their sums. Bars stand
 * on 0, so it is always in; a line may leave it out (`zero: false`), or rates
 * between 10% and 12% would be drawn as one flat line at the top.
 */
export function domain(datasets: ChartDataset[], n: number, stacked = false, zero = true): { min: number; max: number } {
  let min = zero ? 0 : Infinity, max = zero ? 0 : -Infinity;
  if (stacked) {
    for (let i = 0; i < n; i++) {
      let pos = 0, neg = 0;
      for (const ds of datasets) {
        const v = num(ds.values[i]);
        if (v === null) continue;
        if (v >= 0) pos += v; else neg += v;
      }
      max = Math.max(max, pos);
      min = Math.min(min, neg);
    }
  } else {
    for (const ds of datasets) for (let i = 0; i < n; i++) {
      const v = num(ds.values[i]);
      if (v === null) continue;
      max = Math.max(max, v);
      min = Math.min(min, v);
    }
  }
  if (min > max) return { min: 0, max: 0 }; // nothing known
  return { min, max };
}

function niceStep(raw: number): number {
  const exp = Math.pow(10, Math.floor(Math.log10(raw)));
  const f = raw / exp;
  return (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * exp;
}

/** How many decimals a step needs to be printed exactly (0.25 → 2, 5 → 0). */
export function decimalsOf(step: number): number {
  let d = 0;
  while (d < 6 && Math.abs(Math.round(step * 10 ** d) - step * 10 ** d) > 1e-6) d++;
  return d;
}

/**
 * The axis: round ticks (1, 2, 2.5 or 5 times a power of ten) that cover the
 * domain in about four steps, and the scale's ends, which are the outer ticks.
 */
export function scale(min: number, max: number): { min: number; max: number; ticks: number[]; step: number } {
  if (max === min) max = min + 1;
  const step = niceStep((max - min) / 4);
  const lo = Math.floor(min / step + 1e-9) * step;
  const hi = Math.ceil(max / step - 1e-9) * step;
  const ticks: number[] = [];
  for (let i = 0; lo + i * step <= hi + step / 2; i++) ticks.push(Number((lo + i * step).toPrecision(12)));
  return { min: lo, max: ticks[ticks.length - 1], ticks, step };
}

/** The runs of consecutive known points of a series: a line is drawn per run. */
export function lineSegments(values: (number | null)[], n: number): number[][] {
  const runs: number[][] = [];
  let cur: number[] = [];
  for (let i = 0; i < n; i++) {
    if (num(values[i]) === null) {
      if (cur.length) runs.push(cur);
      cur = [];
    } else cur.push(i);
  }
  if (cur.length) runs.push(cur);
  return runs;
}

/**
 * Stacked bars: per series and label, the [low, high] the segment spans, or null
 * when there is nothing to draw. Positives stack up from 0, negatives down.
 */
export function stackBars(datasets: ChartDataset[], n: number): ([number, number] | null)[][] {
  const pos = new Array(n).fill(0), neg = new Array(n).fill(0);
  return datasets.map((ds) => Array.from({ length: n }, (_, i): [number, number] | null => {
    const v = num(ds.values[i]);
    if (v === null || v === 0) return null;
    if (v > 0) { const a = pos[i]; pos[i] += v; return [a, pos[i]]; }
    const a = neg[i]; neg[i] += v; return [neg[i], a];
  }));
}

export interface Slice { index: number; value: number; share: number; start: number; end: number }

/** A pie's slices, clockwise from twelve o'clock; unknown, zero and negative values take none. */
export function pieSlices(values: (number | null)[]): Slice[] {
  const known = values.map((v, index) => ({ index, value: num(v) ?? 0 })).filter((s) => s.value > 0);
  const total = known.reduce((t, s) => t + s.value, 0);
  if (!total) return [];
  let a = -Math.PI / 2;
  return known.map((s) => {
    const share = s.value / total;
    const start = a;
    a += share * 2 * Math.PI;
    return { ...s, share, start, end: a };
  });
}

/** The SVG path of a slice; with an inner radius it is a ring's segment (a donut). */
export function arcPath(cx: number, cy: number, r: number, r0: number, a0: number, a1: number): string {
  // an arc whose ends meet draws nothing: a whole circle is two halves
  if (a1 - a0 >= 2 * Math.PI - 1e-6) return `${arcPath(cx, cy, r, r0, a0, a0 + Math.PI)} ${arcPath(cx, cy, r, r0, a0 + Math.PI, a1)}`;
  const p = (rad: number, a: number) => `${(cx + rad * Math.cos(a)).toFixed(2)} ${(cy + rad * Math.sin(a)).toFixed(2)}`;
  const large = a1 - a0 > Math.PI ? 1 : 0;
  if (r0 <= 0) return `M ${cx} ${cy} L ${p(r, a0)} A ${r} ${r} 0 ${large} 1 ${p(r, a1)} Z`;
  return `M ${p(r, a0)} A ${r} ${r} 0 ${large} 1 ${p(r, a1)} L ${p(r0, a1)} A ${r0} ${r0} 0 ${large} 0 ${p(r0, a0)} Z`;
}

/** An axis tick: as many decimals as the step needs, a currency in its short form ("R$ 7,6 mi"). */
export function formatTick(v: number, format: string | undefined, step: number): string {
  const d = decimalsOf(step);
  if (format === "Currency") return currencyFmt({ notation: "compact", maximumFractionDigits: Math.max(1, d) }).format(v);
  const s = numberFmt({ minimumFractionDigits: d, maximumFractionDigits: d }).format(v);
  return format === "Percent" ? `${s}%` : s;
}

/** A point in the tooltip, formatted like a cell of the chart's (or the series') format. */
export function formatPoint(v: number, format: string | undefined): string {
  return format ? formatValue(v, { fieldtype: format } as any) : formatNumber(v, 2);
}
