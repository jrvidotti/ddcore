<script lang="ts">
  // The notice a form script puts at the top of a form (frm.setBanner): it stays in view while the
  // form scrolls, and its actions are disabled while one of them runs.
  import type { FormBanner } from "$lib/form.svelte";
  import { showError } from "$lib/ui.svelte";

  let { banner }: { banner: FormBanner } = $props();
  let running = $state(false);

  async function run(action: NonNullable<FormBanner["actions"]>[number]) {
    running = true;
    try { await action.onClick(); } catch (e) { showError(e); } finally { running = false; }
  }
</script>

<div class="form-banner {banner.color || "blue"}" role="status">
  <span class="form-banner-text">{banner.message}</span>
  {#each banner.actions || [] as action (action.label)}
    <button type="button" class="btn sm" class:primary={action.primary} disabled={running} onclick={() => run(action)}>{action.label}</button>
  {/each}
</div>

<style>
  .form-banner { position: sticky; top: 0; z-index: 5; display: flex; align-items: center; gap: 10px; flex-wrap: wrap;
    padding: 8px 14px; margin-bottom: 14px; border-radius: 8px; font-size: 13px; background: var(--blue-soft); color: var(--blue-ink); }
  .form-banner-text { flex: 1; min-width: 200px; }
  .form-banner.orange { background: var(--orange-soft); color: var(--orange-ink); }
  .form-banner.red { background: var(--red-soft); color: var(--red-ink); }
  .form-banner.green { background: var(--green-soft); color: var(--green-ink); }
  .form-banner.yellow { background: var(--yellow-soft); color: var(--yellow-ink); }
  .form-banner.gray { background: var(--gray-soft); color: var(--gray-ink); }
  .form-banner.purple { background: var(--purple-soft); color: var(--purple-ink); }
</style>
