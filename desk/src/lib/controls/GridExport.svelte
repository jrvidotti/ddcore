<script lang="ts">
  // The export buttons of a grid: one per format, so neither needs a menu.
  import Icon from "$lib/components/Icon.svelte";
  import { __ } from "$lib/boot.svelte";

  let { onexport }: { onexport: (format: "csv" | "xlsx") => void } = $props();
</script>

<button class="btn sm" title={__("Export CSV")} aria-label={__("Export CSV")} onclick={() => onexport("csv")}><Icon name="download" size={14} />CSV</button>
<button class="btn sm" title={__("Export XLSX")} aria-label={__("Export XLSX")} onclick={() => onexport("xlsx")}><Icon name="download" size={14} />XLSX</button>
