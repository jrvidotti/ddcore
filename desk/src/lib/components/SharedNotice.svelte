<script lang="ts">
  // Above the form and the list of a shared DocType inside a tenant: its rows
  // belong to every tenant, so the server refuses to change them from here.
  // An operator can go back to the platform space, on the same page.
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { platformHref } from "$lib/tenant";
  import { showError } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";

  let { shared }: { shared?: boolean } = $props();

  const tenant = $derived(boot.data?.site?.tenant);
  let busy = $state(false);

  async function toPlatform() {
    if (busy) return;
    busy = true;
    try {
      await api.enterTenant("");
      // everything on screen was read in the space being left
      location.href = platformHref(page.url);
    } catch (e) {
      showError(e);
      busy = false;
    }
  }
</script>

{#if shared && tenant?.id}
  <div class="shared-notice" role="status">
    <Icon name="lock" size={14} />
    <span>{__("Shared by every tenant: read only here")}</span>
    {#if tenant.platform}
      <button class="btn sm" disabled={busy} onclick={toPlatform}><Icon name="shield" size={14} />{__("Go to the platform")}</button>
    {/if}
  </div>
{/if}

<style>
  .shared-notice {
    display: flex; align-items: center; gap: 8px; flex-wrap: wrap; margin: 0 0 12px; padding: 8px 12px;
    background: #ede9fe; color: #5b21b6; border: 1px solid #ddd6fe; border-radius: 6px; font-size: 13px;
  }
  .shared-notice span { flex: 1; min-width: 12em; }
</style>
