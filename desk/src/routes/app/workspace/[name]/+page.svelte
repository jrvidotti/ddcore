<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/Spinner.svelte";
  const name = $derived(page.params.name ?? "");
  $effect(() => {
    if (name) goto(`/app/${encodeURIComponent(name)}`, { replaceState: true });
  });
</script>
<div class="page"><Spinner /></div>
