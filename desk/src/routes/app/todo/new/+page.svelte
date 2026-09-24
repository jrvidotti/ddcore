<script lang="ts">
  // A task created from the To-Do page stays under it, whatever workspace ToDo belongs to.
  import FormView from "$lib/components/FormView.svelte";
</script>
<FormView doctype="ToDo" id="new" basePath="/app/todo" />
