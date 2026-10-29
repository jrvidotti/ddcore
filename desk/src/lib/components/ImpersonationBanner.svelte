<script lang="ts">
  // Shown on every page, desk and portal, while an administrator works as
  // another user: who they are working as, and the way back.
  import { __, boot } from "$lib/boot.svelte";
  import { stopImpersonating } from "$lib/impersonation";
  import { showError } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";

  const by = $derived(boot.data?.impersonatedBy);
  const name = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");
  let stopping = $state(false);

  async function stop() {
    stopping = true;
    try {
      await stopImpersonating();
    } catch (e) {
      stopping = false;
      showError(e);
    }
  }
</script>

{#if by}
  <div class="impersonation-banner" role="status">
    <Icon name="user-check" size={14} />
    <span><strong>{__("Viewing as {0}", [name])}</strong> — {__("what you do is recorded as them, tagged with your name.")}</span>
    <button class="btn" type="button" onclick={stop} disabled={stopping}>{__("Stop impersonating")}</button>
  </div>
{/if}

<style>
  .impersonation-banner {
    display: flex; align-items: center; gap: 8px; padding: 8px 16px; flex-wrap: wrap;
    background: var(--purple-soft); color: var(--purple-ink); border-bottom: 1px solid color-mix(in srgb, var(--purple-ink) 25%, var(--surface)); font-size: 13px;
  }
  .impersonation-banner span { flex: 1; min-width: 0; }
  .impersonation-banner .btn { padding: 2px 10px; font-size: 12px; }
</style>
