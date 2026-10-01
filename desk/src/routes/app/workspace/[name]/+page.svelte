<script lang="ts">
  import { seg } from "$lib/routes";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/Spinner.svelte";
  const name = $derived(page.params.name ?? "");
  $effect(() => {
    if (name) goto(`/app/${seg(name)}${page.url.search}${page.url.hash}`, { replaceState: true });
  });
</script>
<div class="page"><Spinner /></div>
