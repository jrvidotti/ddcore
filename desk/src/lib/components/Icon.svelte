<script lang="ts">
  // A lucide icon from the desk's set ($lib/icons); an unknown name draws a circle.
  import { drawIcon } from "$lib/icons";
  let { name = "", size = 16 }: { name?: string; size?: number } = $props();
  const paths = $derived(drawIcon(name));
</script>

<svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true" style="flex-shrink:0">
  {#each paths as d}
    <path {d} />
  {/each}
</svg>
