<script lang="ts">
  // Hierarchy view (DAT-07): one level at a time, from /api/tree.
  import { api } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import type { Meta } from "$lib/meta";
  import Icon from "../Icon.svelte";
  import { emptyTree, mergeChildren, markLoading, toggle, visibleRows, loadedParents, ROOT, type TreeState } from "./tree-state";

  let { meta, doctype, wsPrefix, reloadKey = 0 }: {
    meta: Meta; doctype: string; wsPrefix: string;
    /** Bumped by the list when a document changed, to reload the open branches. */
    reloadKey?: number;
  } = $props();

  let tree = $state<TreeState>(emptyTree());
  let loading = $state(true);
  const parentField = $derived(meta.doctype.parentField || "");
  const rows = $derived(visibleRows(tree));

  async function loadLevel(parent: string) {
    try {
      const res = await api.treeChildren(doctype, parent);
      tree = mergeChildren(tree, parent, res);
    } catch (e) {
      tree = { ...tree, loading: new Set([...tree.loading].filter((p) => p !== parent)) };
      showError(e);
    }
  }

  async function onToggle(id: string) {
    const next = toggle(tree, id);
    tree = next.state;
    if (next.needsLoad) await loadLevel(id);
  }

  async function reload() {
    loading = true;
    const parents = tree.children.size ? loadedParents(tree) : [ROOT];
    for (const parent of parents) tree = markLoading(tree, parent);
    await Promise.all(parents.map(loadLevel));
    loading = false;
  }

  function childHref(parent: string) {
    const query = parentField ? `?${encodeURIComponent(parentField)}=${encodeURIComponent(parent)}` : "";
    return `${wsPrefix}/${encodeURIComponent(doctype)}/new${query}`;
  }

  $effect(() => {
    reloadKey;
    reload();
  });
</script>

<div class="card tree">
  {#if loading && rows.length === 0}
    <p class="muted empty">{__("Loading…")}</p>
  {:else if rows.length === 0}
    <p class="muted empty">{__("No records")}</p>
  {:else}
    <ul class="level">
      {#each rows as row (row.node.id)}
        <li style={`--depth:${row.depth}`}>
          <div class="row">
            {#if row.expandable}
              <button class="btn icon toggle" onclick={() => onToggle(row.node.id)}
                aria-expanded={row.expanded} aria-label={row.expanded ? __("Collapse") : __("Expand")}>
                <Icon name={row.expanded ? "chevron-down" : "chevron-right"} size={14} />
              </button>
            {:else}
              <span class="toggle spacer" aria-hidden="true"></span>
            {/if}
            <Icon name={row.expandable ? "folder" : "file"} size={14} />
            <a class="title" href={`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(row.node.id)}`}>{row.node.title}</a>
            {#if row.expandable && row.node.children > 0}<span class="muted small count">{row.node.children}</span>{/if}
            {#if row.loading}<span class="muted small">{__("Loading…")}</span>{/if}
            {#if row.expandable && meta.permissions.create}
              <a class="btn sm add" href={childHref(row.node.id)} title={__("Add child")} aria-label={__("Add child")}><Icon name="plus" size={12} /></a>
            {/if}
          </div>
          {#if row.expanded && tree.hasMore.get(row.node.id)}
            <div class="row more muted small" style="--depth:{row.depth + 1}">{__("Showing the first nodes only; open the list view to see the rest")}</div>
          {/if}
        </li>
      {/each}
    </ul>
    {#if tree.hasMore.get(ROOT)}
      <p class="muted small empty">{__("Showing the first nodes only; open the list view to see the rest")}</p>
    {/if}
  {/if}
</div>

<style>
  .tree { padding: 0.5rem 0.25rem; }
  .level { list-style: none; margin: 0; padding: 0; }
  .row { display: flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.5rem; padding-left: calc(0.5rem + var(--depth) * 1.25rem); border-radius: var(--radius); }
  .row:hover { background: var(--bg-subtle, rgba(0, 0, 0, 0.04)); }
  .toggle { width: 1.5rem; }
  .toggle.spacer { display: inline-block; }
  .title { font-weight: 500; }
  .count { margin-left: 0.1rem; }
  .add { margin-left: auto; }
  .more { padding-top: 0; }
  .empty { padding: 1rem; }
</style>
