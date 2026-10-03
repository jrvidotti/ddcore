<script lang="ts">
  // The space the person works in, under the site's name: a tenant, or the
  // platform space in its own color, since that is where a change reaches
  // every tenant. For an operator it opens the tenant menu. It draws nothing
  // without tenancy, nor for a platform user who is not an operator.
  import { boot, spaceLabel, __ } from "$lib/boot.svelte";
  import { spaceKind } from "$lib/tenant";
  import Icon from "./Icon.svelte";
  import TenantMenu from "./TenantMenu.svelte";

  const tenant = $derived(boot.data?.site?.tenant);
  const label = $derived(spaceLabel());
  const kind = $derived(spaceKind(tenant));
  const icon = $derived(kind === "platform" ? "shield" : "building-2");
  let open = $state(false);

  function onPointerDown(e: PointerEvent) {
    if (open && !(e.target as HTMLElement | null)?.closest(".space-badge")) open = false;
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") open = false;
  }
</script>

<svelte:window onpointerdown={onPointerDown} onkeydown={onKeydown} />

{#if label}
  <div class="space-badge dropdown">
    {#if tenant?.platform}
      <button type="button" class="space {kind}" aria-haspopup="menu" aria-expanded={open} title={__("Tenant")} onclick={() => (open = !open)}>
        <Icon name={icon} size={12} /><span>{label}</span><Icon name="chevron-down" size={12} />
      </button>
      {#if open}
        <div class="menu" role="menu"><TenantMenu onpick={() => (open = false)} /></div>
      {/if}
    {:else}
      <span class="space {kind}"><Icon name={icon} size={12} /><span>{label}</span></span>
    {/if}
  </div>
{/if}

<style>
  .space-badge { min-width: 0; }
  .space {
    display: inline-flex; align-items: center; gap: 4px; max-width: 100%; padding: 1px 6px; border: 0; border-radius: 4px;
    font: inherit; font-size: 11px; font-weight: 500; line-height: 16px; white-space: nowrap;
  }
  .space span { overflow: hidden; text-overflow: ellipsis; }
  button.space { cursor: pointer; }
  .space.platform { background: var(--space-platform-bg); color: var(--space-platform-fg); }
  .space.tenant { background: var(--space-tenant-bg); color: var(--space-tenant-fg); }
  .menu { left: 0; right: auto; }
  .menu :global(button) { display: flex; align-items: center; gap: 8px; }
</style>
