<script lang="ts">
  // Minimal SVG bar/line chart: labels + datasets, no dependencies.
  // `data.type` selects the shape: "line" draws a polyline per dataset, anything
  // else draws grouped bars. Both share the scale, the tooltip and the legend.
  import { formatNumber } from "$lib/format";
  let { data, height = 220 }: { data: { type?: string; labels: string[]; datasets: { name: string; values: number[] }[] }; height?: number } = $props();
  const colors = ["#2563eb", "#dc2626", "#16a34a", "#ea580c", "#7c3aed"];
  const W = 800, padL = 60, padB = 28, padT = 10;
  const val = (ds: { values: number[] }, i: number) => Number(ds.values[i]) || 0;
  const isLine = $derived(data.type === "line");
  // the scale must cover negatives too, or a loss would be drawn off the canvas
  const flat = $derived(data.datasets.flatMap((d) => d.values.map((v) => Number(v) || 0)));
  const max = $derived(Math.max(1, ...flat));
  const min = $derived(Math.min(0, ...flat));
  const n = $derived(data.labels.length);
  const groupW = $derived((W - padL) / Math.max(n, 1));
  const barW = $derived(Math.max(4, (groupW * 0.7) / Math.max(data.datasets.length, 1)));
  const y = (v: number) => padT + (height - padB - padT) * (1 - (v - min) / (max - min || 1));
  const cx = (i: number) => padL + groupW * i + groupW / 2;
  const ticks = $derived([0, 0.25, 0.5, 0.75, 1].map((t) => min + (max - min) * t));
  let hover = $state<{ x: number; y: number; text: string } | null>(null);
  const show = (i: number, ds: { name: string; values: number[] }) =>
    (hover = { x: cx(i), y: y(val(ds, i)), text: `${data.labels[i]} · ${ds.name}: ${formatNumber(val(ds, i), 2)}` });
</script>

<div style="position:relative">
  <svg viewBox="0 0 {W} {height}" style="width:100%;height:auto;font-size:11px">
    {#each ticks as t}
      <line x1={padL} x2={W} y1={y(t)} y2={y(t)} stroke="#eee" />
      <text x={padL - 6} y={y(t) + 4} text-anchor="end" fill="#6b7280">{formatNumber(t, 0)}</text>
    {/each}
    {#each data.labels as label, i}
      <text x={cx(i)} y={height - 8} text-anchor="middle" fill="#6b7280">{label}</text>
    {/each}
    {#if isLine}
      {#each data.datasets as ds, j}
        <polyline fill="none" stroke={colors[j % colors.length]} stroke-width="2" stroke-linejoin="round" stroke-linecap="round"
          points={data.labels.map((_, i) => `${cx(i)},${y(val(ds, i))}`).join(" ")} />
        {#each data.labels as label, i}
          <circle cx={cx(i)} cy={y(val(ds, i))} r="3.5" fill="#fff" stroke={colors[j % colors.length]} stroke-width="2" />
          <!-- an invisible, larger target makes the points easy to hover -->
          <circle cx={cx(i)} cy={y(val(ds, i))} r="10" fill="transparent" role="img" aria-label="{ds.name} {label}"
            onmouseenter={() => show(i, ds)} onmouseleave={() => (hover = null)} />
        {/each}
      {/each}
    {:else}
      {#each data.labels as label, i}
        {#each data.datasets as ds, j}
          <rect x={padL + groupW * i + groupW * 0.15 + barW * j} y={Math.min(y(val(ds, i)), y(0))} width={barW} height={Math.max(1, Math.abs(y(val(ds, i)) - y(0)))}
            fill={colors[j % colors.length]} rx="2" role="img" aria-label="{ds.name} {label}"
            onmouseenter={() => show(i, ds)} onmouseleave={() => (hover = null)} />
        {/each}
      {/each}
    {/if}
  </svg>
  {#if hover}<div style="position:absolute;left:{(hover.x / W) * 100}%;top:0;transform:translateX(-50%);background:#1f2937;color:#fff;padding:3px 8px;border-radius:4px;font-size:12px;pointer-events:none;white-space:nowrap">{hover.text}</div>{/if}
  {#if data.datasets.length > 1 || isLine}
    <div style="display:flex;gap:14px;font-size:12px;margin-top:6px;flex-wrap:wrap">
      {#each data.datasets as ds, j}
        <span><span style="display:inline-block;width:10px;height:{isLine ? 3 : 10}px;background:{colors[j % colors.length]};border-radius:2px;margin-right:4px;vertical-align:middle"></span>{ds.name}</span>
      {/each}
    </div>
  {/if}
</div>
