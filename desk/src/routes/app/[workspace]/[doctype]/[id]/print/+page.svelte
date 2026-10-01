<script lang="ts">
  import { page } from "$app/state";
  import { boot } from "$lib/boot.svelte";
  import { resolveDoctype } from "$lib/routes";
  import PrintView from "$lib/components/PrintView.svelte";

  const doctype = $derived(resolveDoctype(page.params.doctype ?? "", boot.data));
  const id = $derived(page.params.id ?? "");
</script>

{#key doctype + "/" + id}
  <PrintView {doctype} {id} />
{/key}
