<script lang="ts">
  // The actions attached to a field (see frm.addFieldButton): beside an input,
  // in a grid's toolbar, or under an HTML field's content.
  import type { FieldButton } from "$lib/form.svelte";
  import Icon from "$lib/components/Icon.svelte";

  let { buttons = [], small = false }: {
    buttons?: FieldButton[];
    /** the size of a grid toolbar's buttons, instead of an input's height */
    small?: boolean;
  } = $props();
</script>

{#if buttons.length}
  <span class="field-btns">
    {#each buttons as b}
      <button type="button" class="btn field-btn" class:sm={small} class:icon={!!b.icon} title={b.label} aria-label={b.icon ? b.label : undefined} onclick={() => b.onClick()}>
        {#if b.icon}<Icon name={b.icon} size={small ? 14 : 16} />{:else}{b.label}{/if}
      </button>
    {/each}
  </span>
{/if}

<style>
  .field-btns { display: inline-flex; align-items: flex-start; gap: 6px; }
  .field-btns :global(.btn.field-btn:not(.sm)) { min-height: 34px; font-size: 12px; }
</style>
