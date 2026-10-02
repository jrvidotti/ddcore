<script lang="ts">
  // The operator's way in and out of a tenant, as items of the user menu. It
  // draws nothing for anyone else, and nothing on a site without tenancy.
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { tenantChoices } from "$lib/tenant";
  import Icon from "./Icon.svelte";

  let { onpick = () => {} }: { onpick?: () => void } = $props();

  const choices = $derived(tenantChoices(boot.data?.site?.tenant, __("Platform")));
  let busy = $state(false);

  async function enter(id: string) {
    if (busy) return;
    busy = true;
    onpick();
    try {
      await api.enterTenant(id);
      // everything on screen was read in the space being left
      location.href = "/app";
    } finally {
      busy = false;
    }
  }
</script>

{#if choices.length > 1}
  <div class="tenant-menu" role="group" aria-label={__("Tenant")}>
    <div class="tenant-heading">{__("Tenant")}</div>
    <div class="tenant-list">
      {#each choices as c (c.id)}
        <button role="menuitemradio" aria-checked={c.current} disabled={busy || c.current} onclick={() => enter(c.id)}>
          <Icon name={c.current ? "check" : c.id === "" ? "shield" : "building-2"} size={14} /> <span>{c.label}</span>
        </button>
      {/each}
    </div>
  </div>
{/if}

<style>
  .tenant-menu { border-bottom: 1px solid var(--border); padding-bottom: 4px; margin-bottom: 4px; }
  .tenant-heading { padding: 6px 12px 2px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); }
  .tenant-list { max-height: 220px; overflow-y: auto; }
  button[aria-checked="true"] { font-weight: 600; opacity: 1; }
</style>
