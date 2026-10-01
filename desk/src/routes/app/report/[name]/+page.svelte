<script lang="ts">
  import { page } from "$app/state";
  import { boot } from "$lib/boot.svelte";
  import { resolveReport } from "$lib/routes";
  import ReportView from "$lib/components/ReportView.svelte";
  const name = $derived(resolveReport(page.params.name ?? "", boot.data));
</script>
{#key name}<ReportView {name} />{/key}
