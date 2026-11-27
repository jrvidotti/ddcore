<script lang="ts">
  // Minimal SVG chart: labels + datasets, no dependencies. `data.type` selects the
  // shape: "line" draws a line per series, "pie" and "donut" draw the first series as
  // slices, "heatmap" draws a matrix (a row per series, a column per label) shaded by value,
  // anything else draws bars (grouped, or stacked with `stacked`). A null value is a point not
  // known yet: the line breaks there and no bar, slice or cell is drawn.
  // With `onpick`, a bar, point or slice whose label `pickable` accepts takes a click (and
  // Enter or Space) and hands over its label's index: a report's drill-down. A heatmap's cell
  // hands over `row * labels.length + col` instead.
  import { arcPath, domain, formatCompact, formatPoint, formatTick, heatFill, heatShade, lineSegments, num, pieSlices, scale, seriesColors, stackBars, type ChartData, type ChartDataset } from "./chart-layout";
  let { data, height = 220, onpick, pickable }: { data: ChartData; height?: number; onpick?: (index: number) => void; pickable?: (index: number) => boolean } = $props();
  const padB = 28, padT = 10;
  // drawn at the width it is shown at, so the text stays 11px in a half-width card;
  // 800 until it is measured (and where nothing is, as in a test)
  let width = $state(0);
  const W = $derived(width || 800);
  const isLine = $derived(data.type === "line");
  const isPie = $derived(data.type === "pie" || data.type === "donut");
  const isHeat = $derived(data.type === "heatmap");
  const stacked = $derived(!isLine && !isPie && !isHeat && !!data.stacked);
  const n = $derived(data.labels.length);
  // a heatmap is as tall as its rows; the other shapes take `height`
  const rowH = 26;
  const H = $derived(isHeat ? padT + data.datasets.length * rowH + padB : height);
  // CSS colours, so they reach the SVG through `style:` (a presentation attribute does not resolve var())
  const colors = $derived(seriesColors(data.datasets.map((d) => d.color)));
  const sliceColors = $derived(seriesColors(data.labels.map(() => undefined)));
  // the scale must cover negatives too, or a loss would be drawn off the canvas
  const sc = $derived.by(() => { const d = domain(data.datasets, n, stacked, !isLine); return scale(d.min, d.max); });
  const tickText = $derived(sc.ticks.map((t) => formatTick(t, data.format, sc.step)));
  // room for the longest tick (≈0.6em per char at 11px), so "R$ 7,6 mi" is not cut
  // (a heatmap's left column holds its row names instead)
  const padL = $derived(Math.max(40, Math.max(0, ...(isHeat ? data.datasets.map((d) => String(d.name).length) : tickText.map((t) => t.length))) * 6.6 + 12));
  const groupW = $derived((W - padL) / Math.max(n, 1));
  const barW = $derived(Math.max(4, (groupW * 0.7) / (stacked ? 1 : Math.max(data.datasets.length, 1))));
  const y = (v: number) => padT + (H - padB - padT) * (1 - (v - sc.min) / (sc.max - sc.min || 1));
  const cx = (i: number) => padL + groupW * i + groupW / 2;
  const stacks = $derived(stacked ? stackBars(data.datasets, n) : []);
  // one label every `labelStep` groups, so they do not overlap; the width is estimated from the
  // longest label (≈0.6em per char at 11px, plus a gap) because SVG text is not measured before it
  // is drawn. Counted from the end, so the latest point of a time series keeps its label.
  const labelW = $derived(Math.max(0, ...data.labels.map((l) => String(l).length)) * 6.6 + 6);
  const labelStep = $derived(Math.max(1, Math.ceil(labelW / groupW)));
  const showLabel = (i: number) => (n - 1 - i) % labelStep === 0;
  // a label wider than its group would be cut at the canvas' right edge: it is pulled in
  const labelX = (i: number, label: string) => Math.min(cx(i), W - (String(label).length * 6.6) / 2 - 2);
  // a pie: the first series, centred, a ring when it is a donut
  const R = $derived((height - padT * 2) / 2);
  const slices = $derived(isPie && data.datasets[0] ? pieSlices(data.datasets[0].values) : []);
  // a heatmap: its scale, from the chart's min and max or the known values', and what each known cell draws
  const heat = $derived.by(() => {
    const d = domain(data.datasets, n, false, false);
    return { min: num(data.min) ?? d.min, max: num(data.max) ?? d.max };
  });
  const cells = $derived(isHeat
    ? data.datasets.flatMap((ds, j) => data.labels.flatMap((_, i) => {
        const v = num(ds.values[i]);
        if (v === null) return [];
        const t = heatShade(v, heat.min, heat.max);
        const text = formatCompact(v, ds.format || data.format);
        // the value is printed when it fits its cell (≈0.6em per char at 11px)
        return [{ i, j, v, t, text: text.length * 6.6 + 6 <= groupW ? text : "" }];
      }))
    : []);
  // where the tooltip stands: over the mark, or (`y`) above a heatmap's cell, below one of the top row
  let hover = $state<{ x: number; y?: number; below?: boolean; text: string } | null>(null);
  // what makes the mark of label `i` take a click; nothing when it does not
  const pick = (i: number) =>
    onpick && (!pickable || pickable(i))
      ? { role: "button", tabindex: 0, class: "chart-pick", onclick: () => onpick(i),
          onkeydown: (e: KeyboardEvent) => { if (e.key === "Enter" || e.key === " ") { e.preventDefault(); onpick(i); } } }
      : {};
  const fmt = (ds: ChartDataset, v: number) => formatPoint(v, ds.format || data.format);
  const show = (i: number, ds: ChartDataset, x = cx(i)) =>
    (hover = { x, text: `${data.labels[i]} · ${ds.name}: ${fmt(ds, num(ds.values[i]) ?? 0)}` });
  const showSlice = (s: (typeof slices)[number]) => {
    const ds = data.datasets[0];
    const mid = (s.start + s.end) / 2;
    hover = { x: W / 2 + R * 0.75 * Math.cos(mid), text: `${data.labels[s.index]}: ${fmt(ds, s.value)} (${formatPoint(s.share * 100, "Percent")})` };
  };
  const showCell = (c: (typeof cells)[number]) => {
    const ds = data.datasets[c.j];
    const top = c.j === 0;
    hover = { x: cx(c.i), y: padT + rowH * c.j + (top ? rowH : 0), below: top, text: `${ds.name} · ${data.labels[c.i]}: ${fmt(ds, c.v)}` };
  };
</script>

{#if data.title}<h3 class="chart-title">{data.title}</h3>{/if}
<div style="position:relative" bind:clientWidth={width}>
  <svg viewBox="0 0 {W} {H}" style="width:100%;height:auto;font-size:11px;display:block">
    {#if isHeat}
      {#each data.datasets as ds, j}
        <text x={padL - 6} y={padT + rowH * j + rowH / 2 + 4} text-anchor="end" style:fill="var(--muted)">{ds.name}</text>
      {/each}
      {#each data.labels as label, i}
        {#if showLabel(i)}<text x={labelX(i, label)} y={H - 8} text-anchor="middle" style:fill="var(--muted)">{label}</text>{/if}
      {/each}
      {#each cells as c (c.j * n + c.i)}
        <rect x={padL + groupW * c.i + 1} y={padT + rowH * c.j + 1} width={Math.max(1, groupW - 2)} height={rowH - 2} rx="2"
          style:fill={heatFill(data.color, c.t)} role="img" aria-label="{data.datasets[c.j].name} {data.labels[c.i]}"
          onmouseenter={() => showCell(c)} onmouseleave={() => (hover = null)} {...pick(c.j * n + c.i)} />
        {#if c.text}
          <text x={cx(c.i)} y={padT + rowH * c.j + rowH / 2 + 4} text-anchor="middle" pointer-events="none"
            style:fill={c.t > 0.55 ? "#fff" : "var(--text)"}>{c.text}</text>
        {/if}
      {/each}
    {:else if isPie}
      {#each slices as s (s.index)}
        <path d={arcPath(W / 2, H / 2, R, data.type === "donut" ? R * 0.6 : 0, s.start, s.end)} style:fill={sliceColors[s.index]}
          style:stroke="var(--surface)" stroke-width="1.5" role="img" aria-label="{data.labels[s.index]}"
          onmouseenter={() => showSlice(s)} onmouseleave={() => (hover = null)} {...pick(s.index)} />
      {/each}
    {:else}
      {#each sc.ticks as t, k}
        <line x1={padL} x2={W} y1={y(t)} y2={y(t)} style:stroke="var(--border)" />
        <text x={padL - 6} y={y(t) + 4} text-anchor="end" style:fill="var(--muted)">{tickText[k]}</text>
      {/each}
      {#each data.labels as label, i}
        {#if showLabel(i)}<text x={labelX(i, label)} y={H - 8} text-anchor="middle" style:fill="var(--muted)">{label}</text>{/if}
      {/each}
      {#if isLine}
        {#each data.datasets as ds, j}
          {#each lineSegments(ds.values, n) as run}
            {#if run.length > 1}
              <polyline fill="none" style:stroke={colors[j]} stroke-width="2" stroke-linejoin="round" stroke-linecap="round"
                points={run.map((i) => `${cx(i)},${y(num(ds.values[i]) ?? 0)}`).join(" ")} />
            {/if}
            {#each run as i}
              <circle cx={cx(i)} cy={y(num(ds.values[i]) ?? 0)} r="3.5" style:fill="var(--surface)" style:stroke={colors[j]} stroke-width="2" />
              <!-- an invisible, larger target makes the points easy to hover -->
              <circle cx={cx(i)} cy={y(num(ds.values[i]) ?? 0)} r="10" fill="transparent" role="img" aria-label="{ds.name} {data.labels[i]}"
                onmouseenter={() => show(i, ds)} onmouseleave={() => (hover = null)} {...pick(i)} />
            {/each}
          {/each}
        {/each}
      {:else if stacked}
        {#each data.datasets as ds, j}
          {#each stacks[j] as seg, i}
            {#if seg}
              <rect x={padL + groupW * i + groupW * 0.15} y={y(seg[1])} width={barW} height={Math.max(1, y(seg[0]) - y(seg[1]))}
                style:fill={colors[j]} role="img" aria-label="{ds.name} {data.labels[i]}"
                onmouseenter={() => show(i, ds)} onmouseleave={() => (hover = null)} {...pick(i)} />
            {/if}
          {/each}
        {/each}
      {:else}
        {#each data.labels as label, i}
          {#each data.datasets as ds, j}
            {@const v = num(ds.values[i])}
            {#if v !== null}
              <rect x={padL + groupW * i + groupW * 0.15 + barW * j} y={Math.min(y(v), y(0))} width={barW} height={Math.max(1, Math.abs(y(v) - y(0)))}
                style:fill={colors[j]} rx="2" role="img" aria-label="{ds.name} {label}"
                onmouseenter={() => show(i, ds)} onmouseleave={() => (hover = null)} {...pick(i)} />
            {/if}
          {/each}
        {/each}
      {/if}
    {/if}
  </svg>
  {#if hover}<div style="position:absolute;left:{(hover.x / W) * 100}%;top:{hover.y === undefined ? 0 : `${hover.y + (hover.below ? 2 : -2)}px`};transform:translate(-50%, {hover.y !== undefined && !hover.below ? '-100%' : '0'});background:#1f2937;color:#fff;padding:3px 8px;border-radius:4px;font-size:12px;pointer-events:none;white-space:nowrap">{hover.text}</div>{/if}
  {#if isHeat}
    {#if cells.length}
      <div class="legend heat-legend">
        <span>{formatPoint(heat.min, data.format)}</span>
        <span class="ramp" style:background="linear-gradient(to right, {heatFill(data.color, 0)}, {heatFill(data.color, 1)})"></span>
        <span>{formatPoint(heat.max, data.format)}</span>
      </div>
    {/if}
  {:else if isPie}
    <div class="legend">
      {#each slices as s (s.index)}
        <span><span class="swatch" style:background={sliceColors[s.index]}></span>{data.labels[s.index]}</span>
      {/each}
    </div>
  {:else if data.datasets.length > 1 || isLine}
    <div class="legend">
      {#each data.datasets as ds, j}
        <span><span class="swatch" style:background={colors[j]} style:height={isLine ? "3px" : "10px"}></span>{ds.name}</span>
      {/each}
    </div>
  {/if}
</div>

<style>
  :global(.chart-pick) { cursor: pointer; }
  :global(.chart-pick:focus-visible) { outline: 2px solid var(--primary); }
  .chart-title { font-size: 14px; margin: 0 0 10px; }
  .legend { display: flex; gap: 14px; font-size: 12px; margin-top: 6px; flex-wrap: wrap; }
  .heat-legend { align-items: center; gap: 8px; justify-content: flex-end; color: var(--muted); }
  .ramp { display: inline-block; width: 120px; height: 10px; border-radius: 2px; }
  .swatch { display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 4px; vertical-align: middle; }
</style>
