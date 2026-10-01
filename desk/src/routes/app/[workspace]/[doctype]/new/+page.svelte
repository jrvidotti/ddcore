<script lang="ts">
  import { page } from "$app/state";
  import { boot } from "$lib/boot.svelte";
  import { resolveDoctype } from "$lib/routes";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(resolveDoctype(page.params.doctype ?? "", boot.data));
</script>
{#key doctype}
  <FormView {doctype} id="new" />
{/key}
