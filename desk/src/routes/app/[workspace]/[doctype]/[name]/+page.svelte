<script lang="ts">
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
  const name = $derived(page.params.name ?? "");
</script>
{#key doctype + "/" + name}
  <FormView {doctype} {name} />
{/key}
