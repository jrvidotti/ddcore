<script lang="ts">
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
</script>
{#key doctype}
  <FormView {doctype} name="new" />
{/key}
