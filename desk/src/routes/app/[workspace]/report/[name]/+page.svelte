<script lang="ts">
  import { page } from "$app/state";
  import ReportView from "$lib/components/ReportView.svelte";
  const name = $derived(page.params.name ?? "");
</script>
{#key name}<ReportView {name} />{/key}
