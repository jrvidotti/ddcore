<script lang="ts">
  // A Rating is a whole number of stars. Clearing it stores null, which is not
  // the same as zero: a report can tell an unrated document from a bad one.
  import { __ } from "$lib/boot.svelte";
  import { clampRating, ratingMax, ratingOnKey, ratingOnPick, ratingStars } from "./rating-state";

  let { field, value, onchange, readOnly = false, id = "" }:
    { field: any; value: any; onchange: (v: any) => void; readOnly?: boolean; id?: string } = $props();

  const max = $derived(ratingMax(field?.options));
  const clamped = $derived(clampRating(value, max));
  const stars = $derived(ratingStars(clamped, max));

  function onkeydown(e: KeyboardEvent) {
    if (readOnly) return;
    const next = ratingOnKey(clamped, e.key, max);
    if (next === undefined) return;
    e.preventDefault();
    onchange(next);
  }
</script>

<div
  {id}
  class="rating"
  class:readonly={readOnly}
  role="slider"
  tabindex={readOnly ? -1 : 0}
  aria-label={field?.label || __("Rating")}
  aria-valuemin="0"
  aria-valuemax={max}
  aria-valuenow={clamped ?? 0}
  aria-readonly={readOnly}
  {onkeydown}
>
  {#each stars as filled, i}
    <button
      type="button"
      class="star"
      class:filled
      disabled={readOnly}
      tabindex="-1"
      aria-label={__("{0} of {1}", [String(i + 1), String(max)])}
      onclick={() => onchange(ratingOnPick(clamped, i + 1))}
    >★</button>
  {/each}
  {#if clamped !== null && !readOnly}
    <button type="button" class="clear" title={__("Clear")} onclick={() => onchange(null)}>×</button>
  {/if}
</div>

<style>
  .rating { display: inline-flex; align-items: center; gap: 1px; }
  .rating:focus-visible { outline: 2px solid var(--accent, #2563eb); outline-offset: 2px; border-radius: 4px; }
  .star {
    border: 0;
    background: none;
    padding: 0 1px;
    font-size: 18px;
    line-height: 1;
    color: var(--border, #cbd5e1);
    cursor: pointer;
  }
  .star.filled { color: #f59e0b; }
  .rating.readonly .star { cursor: default; }
  .clear {
    margin-left: 4px;
    border: 0;
    background: none;
    font-size: 14px;
    color: var(--text-muted, #64748b);
    cursor: pointer;
  }
</style>
