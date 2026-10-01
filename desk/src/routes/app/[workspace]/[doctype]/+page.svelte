<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot } from "$lib/boot.svelte";
  import { resolveWorkspaceForDoctype, workspaceRedirect } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import { resolveDoctype, resolveWorkspace, routeName } from "$lib/routes";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";

  const isWorkspace = $derived(resolveWorkspace(page.params.workspace ?? "", boot.data) !== null);
  // the short route /app/<DocType>/<id>: the first segment is a DocType, the second its record
  const workspace = $derived(resolveDoctype(page.params.workspace ?? "", boot.data));
  const isDocInFirstPos = $derived(boot.data?.doctypes && workspace in boot.data.doctypes);
  const doctype = $derived(!isWorkspace && isDocInFirstPos ? page.params.doctype ?? "" : resolveDoctype(page.params.doctype ?? "", boot.data));

  $effect(() => {
    if (boot.ready && !isWorkspace && isDocInFirstPos) {
      const realWs = resolveWorkspaceForDoctype(workspace, boot.data?.workspaces || [], boot.data?.doctypes);
      if (realWs || page.params.workspace !== routeName(workspace)) {
        goto(workspaceRedirect(realWs, workspace, [page.params.doctype ?? ""], page.url), { replaceState: true });
      }
    }
  });

  const meta = $derived(getMeta(doctype));
</script>

{#key doctype}
  {#await meta then m}
    {#if m.doctype.isSingle}
      <FormView {doctype} id="singleton" />
    {:else}
      <ListView {doctype} />
    {/if}
  {:catch error}
    <div class="page"><div class="card empty">{error.message}</div></div>
  {/await}
{/key}
