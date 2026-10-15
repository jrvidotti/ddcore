<script lang="ts">
  // The list's Columns menu: each user checks the columns they want to see.
  // The visible ones come first, in screen order; checking another adds it at
  // the end. "Reset to default" goes back to what the app declared.
  import { __ } from "$lib/boot.svelte";
  import type { Field } from "$lib/meta";
  import Icon from "../Icon.svelte";
  import { toggleColumn, type ColumnChoice, type ResolvedColumns } from "../list-columns";

  let { resolved, candidates, idLabel, onchange }: {
    resolved: ResolvedColumns; candidates: Field[]; idLabel: string;
    onchange: (choice: ColumnChoice | null) => void;
  } = $props();

  let open = $state(false);
  let root = $state<HTMLElement | null>(null);

  const visible = $derived(new Set(resolved.columns.map((c) => c.fieldname)));
  const others = $derived(candidates.filter((c) => !visible.has(c.fieldname))
    .sort((a, b) => (a.label || a.fieldname || "").localeCompare(b.label || b.fieldname || "")));

  function set(column: string, on: boolean) {
    onchange(toggleColumn(resolved, column, on));
  }
  function onPointerDown(e: PointerEvent) {
    if (open && !root?.contains(e.target as Node)) open = false;
  }
  function onKey(e: KeyboardEvent) {
    if (open && e.key === "Escape") open = false;
  }
</script>

<svelte:window onpointerdown={onPointerDown} onkeydown={onKey} />

<div class="dropdown" bind:this={root}>
  <button class="btn" class:active={resolved.customized} aria-haspopup="menu" aria-expanded={open} onclick={() => (open = !open)} title={__("Choose the columns")}>
    <Icon name="table" size={14} />{__("Columns")}
  </button>
  {#if open}
    <div class="menu columns-menu" role="menu" tabindex="-1" aria-label={__("Columns")}>
      <div class="columns-list">
        {@render item("id", idLabel, resolved.showID)}
        {#each resolved.columns as c (c.fieldname)}{@render item(c.fieldname!, c.label || c.fieldname!, true)}{/each}
        {@render item("modified", __("Modified"), resolved.showModified)}
        {#if others.length}
          <div class="columns-heading">{__("Add a column")}</div>
          {#each others as c (c.fieldname)}{@render item(c.fieldname!, c.label || c.fieldname!, false)}{/each}
        {/if}
      </div>
      <div class="columns-footer">
        <button disabled={!resolved.customized} onclick={() => { open = false; onchange(null); }}><Icon name="rotate-ccw" size={14} /> {__("Reset to default")}</button>
      </div>
    </div>
  {/if}
</div>

{#snippet item(column: string, label: string, checked: boolean)}
  <button role="menuitemcheckbox" aria-checked={checked} onclick={() => set(column, !checked)}>
    <input type="checkbox" tabindex="-1" aria-hidden="true" {checked} /> <span>{label}</span>
  </button>
{/snippet}

<style>
  .columns-menu { min-width: 220px; }
  .columns-list { max-height: 320px; overflow-y: auto; }
  .columns-menu button { display: flex; align-items: center; gap: 8px; }
  .columns-menu input { pointer-events: none; margin: 0; }
  .columns-heading { padding: 8px 10px 2px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); border-top: 1px solid var(--border); margin-top: 4px; }
  .columns-footer { border-top: 1px solid var(--border); margin-top: 4px; padding-top: 4px; }
  .columns-footer button:disabled { opacity: 0.5; cursor: default; }
</style>
