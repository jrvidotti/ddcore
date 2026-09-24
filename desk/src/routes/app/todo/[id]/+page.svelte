<script lang="ts">
  // A task opened from the To-Do page stays under it, whatever workspace ToDo belongs to.
  import { page } from "$app/state";
  import FormView from "$lib/components/FormView.svelte";
  const id = $derived(page.params.id ?? "");
</script>
{#key id}
  <FormView doctype="ToDo" {id} basePath="/app/todo" />
{/key}
