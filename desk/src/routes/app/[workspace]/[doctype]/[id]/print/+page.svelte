<script lang="ts">
  import { page } from "$app/state";
  import PrintView from "$lib/components/PrintView.svelte";

  const doctype = $derived(page.params.doctype ?? "");
  const id = $derived(page.params.id ?? "");
</script>

{#key doctype + "/" + id}
  <PrintView {doctype} {id} />
{/key}
