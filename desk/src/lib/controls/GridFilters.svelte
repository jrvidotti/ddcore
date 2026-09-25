<script lang="ts">
  // The preset filters of a grid: one toggle each, on when pressed.
  import type { GridFilter } from "$lib/meta";

  let { filters, active, ontoggle }: { filters: GridFilter[]; active: Set<number>; ontoggle: (i: number) => void } = $props();
</script>

{#each filters as f, i}
  <button class="btn sm grid-filter" class:active={active.has(i)} aria-pressed={active.has(i)} onclick={() => ontoggle(i)}>{f.label}</button>
{/each}
