<script lang="ts">
  import { boot, __ } from "$lib/boot.svelte";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/Spinner.svelte";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";

  $effect(() => {
    if (boot.ready) {
      const rem = getRememberedWorkspace();
      const target = boot.data?.workspaces?.find((w: any) => w.name.toLowerCase() === rem.toLowerCase())?.name ||
        boot.data?.apps?.map((a: any) => a.desk?.home).find(Boolean) ||
        boot.data?.workspaces?.[0]?.name;
      if (target) {
        goto(`/app/${encodeURIComponent(target)}`, { replaceState: true });
      }
    }
  });
</script>

{#if boot.ready && !boot.data?.workspaces?.length}
  <div class="page"><div class="card empty">{__("No workspace declared. Create an app with ddcore new-app and declare a workspace.")}</div></div>
{:else}
  <div class="page"><Spinner /></div>
{/if}
