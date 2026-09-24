<script lang="ts">
  import { __ } from "$lib/boot.svelte";
  import type { KanbanViewOptions } from "$lib/desk-sdk";
  import { selectLabels, selectOptions, type Meta } from "$lib/meta";
  import { statusColor } from "$lib/format";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { canDragKanban, groupKanbanRows, kanbanColumns, kanbanValue } from "./kanban-state";

  let { rows, meta, doctype, wsPrefix, basePath, kanban, loading = false, movable, onMove }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; kanban: KanbanViewOptions;
    /** where the records live; `${wsPrefix}/<doctype>` unless the page has its own routes */
    basePath?: string;
    loading?: boolean;
    /**
     * Whether cards can move; by default when the field is writable. A page whose
     * `onMove` goes through its own endpoint (the To-Do page) decides for itself.
     */
    movable?: boolean;
    /** Moves a card to another column; the parent updates the rows and saves. */
    onMove: (id: string, value: string) => void;
  } = $props();

  const field = $derived(meta.doctype.fields.find((f) => f.fieldname === kanban.field));
  const writable = $derived(movable ?? (!!meta.permissions.write && !!field && !field.readOnly));
  const columns = $derived(kanbanColumns(rows, kanban.field, {
    options: field ? selectOptions(field) : [],
    labels: field ? selectLabels(field) : [],
    columns: kanban.columns,
  }));
  const groups = $derived(groupKanbanRows(rows, kanban.field));
  const titleField = $derived(kanban.titleField || meta.doctype.titleField || "id");
  const colorField = $derived(meta.doctype.fields.find((f) => f.fieldname === (kanban.colorField || kanban.field)));
  const subtitleField = $derived(kanban.subtitleField ? meta.doctype.fields.find((f) => f.fieldname === kanban.subtitleField) : undefined);

  let dragging = $state("");
  let over = $state<string | null>(null);

  function colorLabel(row: any): string {
    const v = String(row[kanban.colorField!]);
    if (colorField?.fieldtype !== "Select") return v;
    const i = selectOptions(colorField).indexOf(v);
    return i >= 0 ? selectLabels(colorField)[i] : v;
  }
  function subtitle(row: any): string {
    if (!subtitleField) return "";
    const v = row[subtitleField.fieldname!];
    if (v === null || v === undefined || v === "") return "";
    if (subtitleField.fieldtype === "Link") return getLinkTitle(subtitleField.options, v) || String(v);
    return String(v);
  }
  function onDragStart(e: DragEvent, row: any) {
    dragging = row.id;
    e.dataTransfer?.setData("text/plain", row.id);
    if (e.dataTransfer) e.dataTransfer.effectAllowed = "move";
  }
  function onDragOver(e: DragEvent, value: string) {
    if (!dragging) return;
    e.preventDefault();
    over = value;
  }
  function onDrop(e: DragEvent, value: string) {
    e.preventDefault();
    const id = dragging || e.dataTransfer?.getData("text/plain") || "";
    dragging = "";
    over = null;
    const row = rows.find((r) => r.id === id);
    if (row && kanbanValue(row, kanban.field) !== value) onMove(id, value);
  }
</script>

<div class="kanban" aria-busy={loading}>
  {#if !field}
    <div class="card empty">{__("The Kanban field {0} is not a field of {1}", [kanban.field, doctype])}</div>
  {/if}
  {#each columns as column (column.value)}
    {@const cards = groups.get(column.value) || []}
    <section
      class="column"
      class:over={over === column.value}
      aria-label={column.value === "" ? __("(empty)") : column.label}
      ondragover={(e) => onDragOver(e, column.value)}
      ondragleave={() => { if (over === column.value) over = null; }}
      ondrop={(e) => onDrop(e, column.value)}
    >
      <header>
        {#if column.value === ""}
          <span class="muted">{__("(empty)")}</span>
        {:else}
          <span class="indicator {statusColor(column.value, field)}">{column.label}</span>
        {/if}
        <span class="muted small">{cards.length}</span>
      </header>
      <div class="cards">
        {#each cards as row (row.id)}
          {@const draggable = canDragKanban(row, writable)}
          <a
            class="kanban-card"
            class:dragging={dragging === row.id}
            class:locked={!draggable}
            href={`${basePath || `${wsPrefix}/${encodeURIComponent(doctype)}`}/${encodeURIComponent(row.id)}`}
            draggable={draggable ? "true" : "false"}
            ondragstart={(e) => onDragStart(e, row)}
            ondragend={() => { dragging = ""; over = null; }}
          >
            <span class="title">{row[titleField] || row.id}</span>
            {#if subtitle(row)}<span class="muted small">{subtitle(row)}</span>{/if}
            {#if kanban.colorField && kanban.colorField !== kanban.field && row[kanban.colorField]}
              <span class="indicator {statusColor(row[kanban.colorField], colorField)} small">{colorLabel(row)}</span>
            {/if}
          </a>
        {/each}
      </div>
    </section>
  {/each}
</div>

<style>
  .kanban { display: flex; gap: 12px; overflow-x: auto; padding-bottom: 8px; align-items: flex-start; }
  .column { flex: 0 0 272px; background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius); display: flex; flex-direction: column; max-height: calc(100vh - 260px); }
  .column.over { border-color: var(--primary); background: #eff6ff; }
  header { display: flex; align-items: center; justify-content: space-between; gap: 8px; padding: 10px 12px; }
  .cards { display: grid; gap: 8px; padding: 0 8px 8px; overflow-y: auto; min-height: 48px; }
  .kanban-card { display: grid; gap: 4px; padding: 10px 12px; background: var(--surface); border: 1px solid var(--border); border-radius: var(--radius); color: var(--text); text-decoration: none; cursor: grab; }
  .kanban-card:hover { border-color: var(--primary); text-decoration: none; }
  .kanban-card.locked { cursor: pointer; }
  .kanban-card.dragging { opacity: .5; }
  .kanban-card .title { font-weight: 500; overflow: hidden; text-overflow: ellipsis; }
  .kanban-card .indicator { justify-self: start; }
  .empty { padding: 16px; }
  @media (max-width: 600px) { .column { flex-basis: 85vw; } }
</style>
