<script lang="ts">
  import { page } from "$app/state";
  import PrintView from "$lib/components/PrintView.svelte";

  const doctype = $derived(page.params.doctype ?? "");
  const name = $derived(page.params.name ?? "");
</script>

{#key doctype + "/" + name}
  <PrintView {doctype} {name} />
{/key}
