<script lang="ts">
  import { page } from "$app/state";
  import Workspace from "$lib/components/Workspace.svelte";
  const name = $derived(page.params.name ?? "");
</script>
{#key name}<Workspace {name} />{/key}
