<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot, __ } from "$lib/boot.svelte";
  import { resolveWorkspaceForDoctype } from "$lib/components/sidebar-workspace";
  import Workspace from "$lib/components/Workspace.svelte";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";
  import { getMeta } from "$lib/meta";

  const segment = $derived(page.params.workspace ?? "");
  const ws = $derived(boot.data?.workspaces?.find((w: any) => w.name.toLowerCase() === segment.toLowerCase()));
  const isDoctype = $derived(boot.data?.doctypes && segment in boot.data.doctypes);

  $effect(() => {
    if (boot.ready && !ws && isDoctype) {
      const realWs = resolveWorkspaceForDoctype(segment, boot.data?.workspaces || [], boot.data?.doctypes);
      if (realWs) {
        goto(`/app/${encodeURIComponent(realWs)}/${encodeURIComponent(segment)}`, { replaceState: true });
      }
    }
  });
</script>

{#if ws}
  {#key ws.name}
    <Workspace name={ws.name} />
  {/key}
{:else if isDoctype}
  {#key segment}
    {#await getMeta(segment) then m}
      {#if m.doctype.isSingle}
        <FormView doctype={segment} name="singleton" />
      {:else}
        <ListView doctype={segment} />
      {/if}
    {:catch error}
      <div class="page"><div class="card empty">{error.message}</div></div>
    {/await}
  {/key}
{:else if boot.ready}
  <div class="page"><div class="card empty">{__("Workspace \"{0}\" not found.", [segment])}</div></div>
{/if}
