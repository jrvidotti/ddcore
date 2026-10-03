<script lang="ts">
  // Drawn in place of a page whose link names another tenant (`?tenant=`): the
  // page would read, and could write, in the wrong space. An operator may enter
  // that tenant — which moves the whole session, every tab with it, so it is
  // asked rather than done — or stay; anyone else learns the link is not theirs.
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { tenantLabel } from "$lib/tenant";
  import { showError } from "$lib/ui.svelte";

  let { kind, tenant }: { kind: "enter" | "foreign"; tenant: string } = $props();

  const current = $derived(boot.data?.site?.tenant);
  const here = $derived(tenantLabel(current, __("Platform")));
  const there = $derived(current?.tenants?.find((x) => x.id === tenant)?.title || tenant);
  let busy = $state(false);

  async function enter() {
    if (busy) return;
    busy = true;
    try {
      await api.enterTenant(tenant);
      // everything already loaded was read in the space being left
      location.reload();
    } catch (e) {
      showError(e);
      busy = false;
    }
  }

  function stay() {
    const url = new URL(page.url);
    url.searchParams.delete("tenant");
    goto(url.pathname + url.search + url.hash, { replaceState: true });
  }
</script>

<div class="page">
  <div class="card empty tenant-mismatch" role="alert">
    {#if kind === "enter"}
      <p>{__("This link belongs to tenant {0}. You are working in {1}.", [there, here])}</p>
      <div class="actions">
        <button class="btn primary" disabled={busy} onclick={enter}>{__("Enter {0}", [there])}</button>
        <button class="btn" disabled={busy} onclick={stay}>{__("Stay in {0}", [here])}</button>
      </div>
    {:else}
      <p>{__("This link belongs to another tenant ({0}) and cannot be opened here.", [tenant])}</p>
      <div class="actions"><a class="btn" href="/app">{__("Go to the desk")}</a></div>
    {/if}
  </div>
</div>

<style>
  .tenant-mismatch p { margin: 0 0 16px; }
  .actions { display: flex; gap: 8px; justify-content: center; flex-wrap: wrap; }
</style>
