<script lang="ts">
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
  const id = $derived(page.params.id ?? "");
</script>
{#key doctype + "/" + id}
  <FormView {doctype} {id} />
{/key}
