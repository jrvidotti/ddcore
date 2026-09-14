<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot } from "$lib/boot.svelte";
  import { resolveWorkspaceForDoctype } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";

  const workspace = $derived(page.params.workspace ?? "");
  const doctype = $derived(page.params.doctype ?? "");

  const isWorkspace = $derived(boot.data?.workspaces?.some((w: any) => w.name.toLowerCase() === workspace.toLowerCase()));
  const isDocInFirstPos = $derived(boot.data?.doctypes && workspace in boot.data.doctypes);

  $effect(() => {
    if (boot.ready && !isWorkspace && isDocInFirstPos) {
      const realWs = resolveWorkspaceForDoctype(workspace, boot.data?.workspaces || [], boot.data?.doctypes);
      if (realWs) {
        goto(`/app/${encodeURIComponent(realWs)}/${encodeURIComponent(workspace)}/${encodeURIComponent(doctype)}`, { replaceState: true });
      }
    }
  });

  const meta = $derived(getMeta(doctype));
</script>

{#key doctype}
  {#await meta then m}
    {#if m.doctype.isSingle}
      <FormView {doctype} name="singleton" />
    {:else}
      <ListView {doctype} />
    {/if}
  {:catch error}
    <div class="page"><div class="card empty">{error.message}</div></div>
  {/await}
{/key}
