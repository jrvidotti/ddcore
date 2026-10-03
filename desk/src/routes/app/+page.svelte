<script lang="ts">
  import { seg } from "$lib/routes";
  import { boot, __ } from "$lib/boot.svelte";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/Spinner.svelte";
  import { getRememberedWorkspace, landingWorkspace } from "$lib/components/sidebar-workspace";

  $effect(() => {
    if (boot.ready) {
      const target = landingWorkspace(boot.data?.workspaces || [], getRememberedWorkspace(), boot.data?.site?.home);
      if (target) {
        goto(`/app/${seg(target)}`, { replaceState: true });
      }
    }
  });
</script>

{#if boot.ready && !boot.data?.workspaces?.length}
  <div class="page"><div class="card empty">{__("No workspace declared. Create an app with ddcore new-app and declare a workspace.")}</div></div>
{:else}
  <div class="page"><Spinner /></div>
{/if}
