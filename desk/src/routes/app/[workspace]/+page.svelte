<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot, __ } from "$lib/boot.svelte";
  import { shortRouteRedirect } from "$lib/components/sidebar-workspace";
  import Workspace from "$lib/components/Workspace.svelte";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";
  import { getMeta } from "$lib/meta";
  import { resolveDoctype, resolveWorkspace } from "$lib/routes";

  const param = $derived(page.params.workspace ?? "");
  const wsName = $derived(resolveWorkspace(param, boot.data));
  const ws = $derived(boot.data?.workspaces?.find((w: any) => w.name === wsName));
  // the short route: the first segment is a DocType, under its route name or its real one
  const segment = $derived(resolveDoctype(param, boot.data));
  const isDoctype = $derived(boot.data?.doctypes && segment in boot.data.doctypes);

  // decided before anything mounts: a form rendered here and then redirected
  // away would run its setup/onload for nothing (#156)
  const redirect = $derived(boot.ready ? shortRouteRedirect(param, [], boot.data, page.url) : null);

  $effect(() => {
    if (redirect) goto(redirect, { replaceState: true });
  });
</script>

{#if ws}
  {#key ws.name}
    <Workspace name={ws.name} />
  {/key}
{:else if redirect || (isDoctype && !boot.ready)}
  <!-- the short route is on its way to its workspace's route -->
{:else if isDoctype}
  {#key segment}
    {#await getMeta(segment) then m}
      {#if m.doctype.isSingle}
        <FormView doctype={segment} id="singleton" />
      {:else}
        <ListView doctype={segment} />
      {/if}
    {:catch error}
      <div class="page"><div class="card empty">{error.message}</div></div>
    {/await}
  {/key}
{:else if boot.ready}
  <div class="page"><div class="card empty">{__("Workspace \"{0}\" not found.", [param])}</div></div>
{/if}
