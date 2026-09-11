<script lang="ts">
  import { page } from "$app/state";
  import { getMeta } from "$lib/meta";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";
  const doctype = $derived(page.params.doctype ?? "");
  const meta = $derived(getMeta(doctype));
</script>
{#key doctype}
  {#await meta then m}
    {#if m.doctype.isSingle}
      <FormView {doctype} name="singleton" />
    {:else}
      <ListView {doctype} />
    {/if}
  {:catch error}
    <p class="error">{error.message}</p>
  {/await}
{/key}
