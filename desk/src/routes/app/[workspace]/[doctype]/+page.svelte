<script lang="ts">
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { boot } from "$lib/boot.svelte";
  import { shortRouteRedirect } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import { resolveDoctype, resolveWorkspace } from "$lib/routes";
  import ListView from "$lib/components/ListView.svelte";
  import FormView from "$lib/components/FormView.svelte";

  const isWorkspace = $derived(resolveWorkspace(page.params.workspace ?? "", boot.data) !== null);
  // the short route /app/<DocType>/<id>: the first segment is a DocType, the second its record
  const workspace = $derived(resolveDoctype(page.params.workspace ?? "", boot.data));
  const isDocInFirstPos = $derived(boot.data?.doctypes && workspace in boot.data.doctypes);
  /** the short route that stays put (a DocType no workspace owns): the second segment is a record id or `new` */
  const shortRecord = $derived(!isWorkspace && !!isDocInFirstPos);
  const doctype = $derived(shortRecord ? workspace : resolveDoctype(page.params.doctype ?? "", boot.data));

  // decided before anything mounts: a form or list rendered here and then
  // redirected away would run its hooks for nothing (#156)
  const redirect = $derived(boot.ready ? shortRouteRedirect(page.params.workspace ?? "", [page.params.doctype ?? ""], boot.data, page.url) : null);

  $effect(() => {
    if (redirect) goto(redirect, { replaceState: true });
  });

  const meta = $derived(getMeta(doctype));
</script>

{#if redirect || !boot.ready}
  <!-- the short route is on its way to its workspace's route -->
{:else if shortRecord}
  {#key doctype + "/" + page.params.doctype}
    <FormView {doctype} id={page.params.doctype ?? ""} />
  {/key}
{:else}
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
{/if}
