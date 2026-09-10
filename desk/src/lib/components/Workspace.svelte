<script lang="ts">
  import { boot, __ } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { formatCurrency, formatNumber } from "$lib/format";
  import BarChart from "./BarChart.svelte";
  import Icon from "./Icon.svelte";
  import { onMount } from "svelte";

  let { name }: { name: string } = $props();
  const ws = $derived(boot.data?.workspaces.find((w: any) => w.name === name));
  let cards = $state<Record<string, any>>({});
  let charts = $state<Record<string, any>>({});

  onMount(async () => {
    if (!ws) return;
    for (const c of ws.numberCards || []) api.numberCard(ws.name, c.name).then((v) => (cards[c.name] = v)).catch((e) => (cards[c.name] = { error: e.message }));
    for (const c of ws.charts || []) api.chart(ws.name, c.name).then((v) => (charts[c.name] = v)).catch((e) => (charts[c.name] = { error: e.message }));
  });
  const fmt = (card: any, v: any) => {
    if (!v) return "…";
    if (v.error) return "!";
    if (v.formatted) return v.formatted;
    if (String(card.aggregate || "").startsWith("sum:")) return formatCurrency(v.value);
    return formatNumber(v.value);
  };
  const href = (s: any) => s.route || (s.doctype ? `/app/${encodeURIComponent(s.doctype)}` : s.report ? `/app/report/${encodeURIComponent(s.report)}` : "#");
</script>

{#if ws}
  <div class="page">
    <div class="page-head"><h1>{ws.label || ws.name}</h1></div>
    {#if ws.numberCards?.length}
      <div class="cards">
        {#each ws.numberCards as c}
          <a class="card ncard" href={c.route || "#"} style="--c: var(--{c.color || 'blue'})">
            <div class="l">{c.label}</div>
            <div class="v">{fmt(c, cards[c.name])}</div>
          </a>
        {/each}
      </div>
    {/if}
    {#each ws.charts || [] as c}
      <div class="card" style="padding:16px 20px;margin-bottom:16px">
        <h3 style="font-size:14px;margin-bottom:10px">{c.label}</h3>
        {#if charts[c.name]?.labels}<BarChart data={charts[c.name]} />{:else if charts[c.name]?.error}<div class="muted">{charts[c.name].error}</div>{:else}<div class="muted">…</div>{/if}
      </div>
    {/each}
    {#if ws.shortcuts?.length}
      <h3 class="sub">{__("Shortcuts")}</h3>
      <div class="shortcuts">
        {#each ws.shortcuts as s}<a class="card sc" href={href(s)}><Icon name={s.icon || "circle"} /> {s.label}</a>{/each}
      </div>
    {/if}
    {#if ws.links?.length}
      <div class="links">
        {#each ws.links as g}
          <div class="card" style="padding:14px 18px">
            <h3 style="font-size:13px;margin-bottom:8px">{g.label}</h3>
            {#each g.items as it}<div><a href={href(it)}>{it.label}</a></div>{/each}
          </div>
        {/each}
      </div>
    {/if}
  </div>
{:else}
  <div class="page muted">{__("Workspace not found")}</div>
{/if}

<style>
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 12px; margin-bottom: 16px; }
  .ncard { padding: 14px 16px; border-left: 4px solid var(--c); color: inherit; text-decoration: none; }
  .ncard .l { font-size: 12px; color: var(--muted); } .ncard .v { font-size: 22px; font-weight: 600; margin-top: 4px; }
  .sub { font-size: 13px; text-transform: uppercase; color: var(--muted); letter-spacing: .04em; margin: 8px 0 10px; }
  .shortcuts { display: grid; grid-template-columns: repeat(auto-fill, minmax(180px, 1fr)); gap: 10px; margin-bottom: 20px; }
  .sc { padding: 12px 14px; display: flex; align-items: center; gap: 10px; color: inherit; font-weight: 500; }
  .sc:hover { text-decoration: none; border-color: var(--primary); }
  .links { display: grid; grid-template-columns: repeat(auto-fill, minmax(240px, 1fr)); gap: 12px; }
  .links div a { display: inline-block; padding: 3px 0; color: var(--text); }
</style>
