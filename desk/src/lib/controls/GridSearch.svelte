<script lang="ts">
  // The search box of a grid (gridSearch): the text narrows the rows on screen.
  import { __ } from "$lib/boot.svelte";

  let { value = $bindable("") }: { value?: string } = $props();
</script>

<input type="search" class="input grid-search" aria-label={__("Search")} placeholder={__("Search…")} bind:value />
