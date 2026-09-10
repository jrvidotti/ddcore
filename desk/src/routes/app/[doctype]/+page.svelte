<script lang="ts">
  import { page } from "$app/state";
  import ListView from "$lib/components/ListView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
</script>
{#key doctype}
  <ListView {doctype} />
{/key}
