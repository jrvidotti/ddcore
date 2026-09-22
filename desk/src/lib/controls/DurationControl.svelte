<script lang="ts">
  // A Duration is whole seconds. In a form it is one box per unit, because
  // "2h 30m" is how the value is thought about; in a grid it is a single text
  // box, because a cell has no room for four.
  import { __ } from "$lib/boot.svelte";
  import { formatDuration, joinDuration, parseDuration, splitDuration, type DurationOptions } from "./duration-format";

  let { field, value, onchange, readOnly = false, error = "", id = "", inGrid = false }:
    { field: any; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string; inGrid?: boolean } = $props();

  const opts = $derived<DurationOptions>({
    hideDays: hasFlag("hideDays"),
    hideSeconds: hasFlag("hideSeconds"),
  });
  function hasFlag(flag: string): boolean {
    const o = field?.options;
    return Array.isArray(o) ? o.includes(flag) : o === flag;
  }

  const parts = $derived(splitDuration(Number(value) || 0, opts));
  let text = $state("");
  $effect(() => { text = formatDuration(value, opts); });

  function setPart(key: "days" | "hours" | "minutes" | "seconds", raw: string) {
    onchange(joinDuration({ ...parts, [key]: Math.max(0, Math.floor(Number(raw) || 0)) }));
  }
</script>

{#if inGrid || readOnly}
  <input
    {id}
    type="text"
    class="input"
    class:error={!!error}
    readonly={readOnly}
    value={text}
    placeholder={opts.hideSeconds ? "1h 30m" : "1h 30m 15s"}
    oninput={(e) => (text = (e.target as HTMLInputElement).value)}
    onblur={(e) => onchange(parseDuration((e.target as HTMLInputElement).value, opts))}
    onkeydown={(e) => e.key === "Enter" && onchange(parseDuration((e.currentTarget as HTMLInputElement).value, opts))}
  />
{:else}
  <div class="duration">
    {#if !opts.hideDays}
      <label class="unit"><input type="number" min="0" class="input num" value={parts.days} onchange={(e) => setPart("days", (e.target as HTMLInputElement).value)} /><span>{__("d")}</span></label>
    {/if}
    <label class="unit"><input {id} type="number" min="0" class="input num" value={parts.hours} onchange={(e) => setPart("hours", (e.target as HTMLInputElement).value)} /><span>{__("h")}</span></label>
    <label class="unit"><input type="number" min="0" max="59" class="input num" value={parts.minutes} onchange={(e) => setPart("minutes", (e.target as HTMLInputElement).value)} /><span>{__("m")}</span></label>
    {#if !opts.hideSeconds}
      <label class="unit"><input type="number" min="0" max="59" class="input num" value={parts.seconds} onchange={(e) => setPart("seconds", (e.target as HTMLInputElement).value)} /><span>{__("s")}</span></label>
    {/if}
    {#if value !== null && value !== undefined}
      <button type="button" class="btn sm" onclick={() => onchange(null)}>{__("Clear")}</button>
    {/if}
  </div>
{/if}

<style>
  .duration { display: flex; align-items: center; gap: 6px; flex-wrap: wrap; }
  .unit { display: inline-flex; align-items: center; gap: 3px; }
  .unit input { width: 58px; text-align: right; }
  .unit span { font-size: 12px; color: var(--text-muted, #64748b); }
</style>
