<script lang="ts">
  // The actions attached to a field (see frm.addFieldButton): beside an input,
  // in a grid's toolbar, or under an HTML field's content. A button whose
  // onClick returns a promise stays disabled, with a spinner, until it settles.
  import type { FieldButton } from "$lib/form.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { showError } from "$lib/ui.svelte";

  let { buttons = [], small = false, disabled = false }: {
    buttons?: FieldButton[];
    /** the size of a grid toolbar's buttons, instead of an input's height */
    small?: boolean;
    /** every button off, e.g. while the grid they act on is loading */
    disabled?: boolean;
  } = $props();
  let running = $state<Set<FieldButton>>(new Set());

  async function run(b: FieldButton) {
    running = new Set(running).add(b);
    try {
      await b.onClick();
    } catch (e) { showError(e); }
    finally { const s = new Set(running); s.delete(b); running = s; }
  }
</script>

{#if buttons.length}
  <span class="field-btns">
    {#each buttons as b}
      {@const busy = running.has(b)}
      <button type="button" class="btn field-btn" class:sm={small} class:icon={!!b.icon} title={b.label} aria-label={b.icon ? b.label : undefined} aria-busy={busy || undefined} disabled={busy || disabled} onclick={() => run(b)}>
        {#if busy}<span class="btn-spin" style:width="{small ? 14 : 16}px" style:height="{small ? 14 : 16}px" aria-hidden="true"></span>{/if}
        {#if b.icon}{#if !busy}<Icon name={b.icon} size={small ? 14 : 16} />{/if}{:else}{b.label}{/if}
      </button>
    {/each}
  </span>
{/if}

<style>
  .field-btns { display: inline-flex; align-items: flex-start; gap: 6px; }
  .field-btns :global(.btn.field-btn:not(.sm)) { min-height: 34px; font-size: 12px; }
  .btn-spin {
    display: inline-block; box-sizing: border-box; border-radius: 50%;
    border: 2px solid currentColor; border-top-color: transparent;
    animation: field-btn-spin 0.8s linear infinite;
  }
  @keyframes field-btn-spin { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .btn-spin { animation-duration: 2.4s; } }
</style>
