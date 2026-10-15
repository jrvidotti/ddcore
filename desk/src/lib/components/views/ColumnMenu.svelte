<script lang="ts">
  // The list's Columns menu: each user checks the columns they want to see.
  // The visible ones come first, in screen order, and are dragged (or moved
  // with Alt+Up/Down) into another; checking another adds it at the end.
  // "Reset to default" goes back to what the app declared.
  import { tick } from "svelte";
  import { __ } from "$lib/boot.svelte";
  import type { Field } from "$lib/meta";
  import Icon from "../Icon.svelte";
  import { moveColumn, toggleColumn, type ColumnChoice, type ResolvedColumns } from "../list-columns";

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

  // The visible column being dragged, and the gap it would drop into: before
  // the column at `slot` (resolved.columns.length is after the last one).
  let dragging = $state<number | null>(null);
  let slot = $state<number | null>(null);

  function move(from: number, to: number) {
    if (from !== to) onchange(moveColumn(resolved, from, to));
  }
  function onDragStart(e: DragEvent, i: number) {
    dragging = i;
    e.dataTransfer?.setData("text/plain", resolved.columns[i].fieldname!);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
  }
  function onDragOver(e: DragEvent, i: number) {
    if (dragging === null) return;
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = "move";
    const box = (e.currentTarget as HTMLElement).getBoundingClientRect();
    slot = e.clientY < box.top + box.height / 2 ? i : i + 1;
  }
  function onDrop(e: DragEvent) {
    e.preventDefault();
    if (dragging !== null && slot !== null) move(dragging, slot > dragging ? slot - 1 : slot);
    onDragEnd();
  }
  function onDragEnd() {
    dragging = null;
    slot = null;
  }
  async function onItemKey(e: KeyboardEvent, i: number) {
    if (!e.altKey || (e.key !== "ArrowUp" && e.key !== "ArrowDown")) return;
    e.preventDefault();
    const to = e.key === "ArrowUp" ? i - 1 : i + 1;
    if (to < 0 || to >= resolved.columns.length) return;
    const column = resolved.columns[i].fieldname!;
    move(i, to);
    // the keyed item moved in the DOM, which can drop its focus
    await tick();
    root?.querySelector<HTMLElement>(`[data-column="${CSS.escape(column)}"]`)?.focus();
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
        {#each resolved.columns as c, i (c.fieldname)}
          <button role="menuitemcheckbox" aria-checked="true" data-column={c.fieldname} draggable="true"
            class:dragging={dragging === i} class:drop-before={slot === i && dragging !== null && dragging !== i && dragging !== i - 1}
            class:drop-after={slot === i + 1 && i === resolved.columns.length - 1 && dragging !== null && dragging !== i}
            title={__("Drag to reorder (Alt+Up/Down)")}
            onclick={() => set(c.fieldname!, false)} onkeydown={(e) => onItemKey(e, i)}
            ondragstart={(e) => onDragStart(e, i)} ondragover={(e) => onDragOver(e, i)} ondrop={onDrop} ondragend={onDragEnd}>
            <input type="checkbox" tabindex="-1" aria-hidden="true" checked /> <span class="label">{c.label || c.fieldname}</span>
            <span class="grip" aria-hidden="true"><Icon name="grip-vertical" size={14} /></span>
          </button>
        {/each}
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
  .columns-menu .label { flex: 1; }
  .grip { display: flex; color: var(--muted); cursor: grab; }
  .columns-menu button[draggable="true"] { border-top: 2px solid transparent; border-bottom: 2px solid transparent; }
  .columns-menu button.dragging { opacity: .5; }
  .columns-menu button.drop-before { border-top-color: var(--primary); }
  .columns-menu button.drop-after { border-bottom-color: var(--primary); }
  .columns-heading { padding: 8px 10px 2px; font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--muted); border-top: 1px solid var(--border); margin-top: 4px; }
  .columns-footer { border-top: 1px solid var(--border); margin-top: 4px; padding-top: 4px; }
  .columns-footer button:disabled { opacity: 0.5; cursor: default; }
</style>
