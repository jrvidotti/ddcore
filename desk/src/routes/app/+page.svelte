<script lang="ts">
  import { boot } from "$lib/boot.svelte";
  import { goto } from "$app/navigation";
  import { onMount } from "svelte";
  import Workspace from "$lib/components/Workspace.svelte";
  const home = $derived(boot.data?.apps.map((a) => a.desk?.home).find(Boolean) || boot.data?.workspaces[0]?.name);
</script>

{#if home}
  <Workspace name={home} />
{:else}
  <div class="page"><div class="card empty">Nenhum workspace. Crie um app com <span class="kbd">ddcore new-app</span> e declare um workspace.</div></div>
{/if}
