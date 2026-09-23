<script lang="ts">
  // Hierarchy view (DAT-07): one level at a time, from /api/tree.
  import { untrack } from "svelte";
  import { api } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import type { Meta } from "$lib/meta";
  import Icon from "../Icon.svelte";
  import { emptyTree, mergeChildren, markLoading, toggle, visibleRows, loadedParents, expandAll, collapseAll, treeFields, treeLabel, rememberTreeExpand, getRememberedTreeExpand, ROOT, type TreeState, type TreeViewSettings } from "./tree-state";

  let { meta, doctype, wsPrefix, reloadKey = 0, settings }: {
    meta: Meta; doctype: string; wsPrefix: string;
    /** Bumped by the list when a document changed, to reload the open branches. */
    reloadKey?: number;
    /** The `tree` option of the DocType's `defineListView`. */
    settings?: TreeViewSettings;
  } = $props();

  let tree = $state<TreeState>(emptyTree());
  let loading = $state(true);
  const parentField = $derived(meta.doctype.parentField || "");
  const rows = $derived(visibleRows(tree));

  async function loadLevel(parent: string) {
    try {
      const res = await api.treeChildren(doctype, parent, undefined, {
        fields: treeFields(settings, meta.doctype.titleField), orderBy: settings?.orderBy,
      });
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
    const first = tree.children.size === 0;
    const parents = first ? [ROOT] : loadedParents(tree);
    for (const parent of parents) tree = markLoading(tree, parent);
    await Promise.all(parents.map(loadLevel));
    loading = false;
    // Only the first load applies the remembered choice: a later reload keeps
    // whatever the user opened or closed since.
    if (first && getRememberedTreeExpand(doctype) === "expanded") await onExpandAll();
  }

  let expanding = $state(false);

  /** Fetches the tree one level per round until every group is open. */
  async function onExpandAll() {
    expanding = true;
    // a branch that failed to load is not asked for again in this round-trip
    const attempted = new Set<string>();
    try {
      for (;;) {
        const step = expandAll(tree);
        tree = step.state;
        const toLoad = step.toLoad.filter((id) => !attempted.has(id));
        if (toLoad.length === 0) break;
        for (const id of toLoad) attempted.add(id);
        await Promise.all(toLoad.map(loadLevel));
      }
    } finally {
      expanding = false;
    }
  }

  function onCollapseAll() {
    rememberTreeExpand(doctype, "collapsed");
    tree = collapseAll(tree);
  }

  function childHref(parent: string) {
    const query = parentField ? `?${encodeURIComponent(parentField)}=${encodeURIComponent(parent)}` : "";
    return `${wsPrefix}/${encodeURIComponent(doctype)}/new${query}`;
  }

  // Only `reloadKey` may re-run this: reload() reads `tree`, and tracking it
  // would re-run the effect on every level it loads, forever.
  $effect(() => {
    reloadKey;
    untrack(reload);
  });
</script>

<div class="card tree">
  {#if rows.length > 0}
    <div class="bar">
      <button class="btn sm" onclick={() => { rememberTreeExpand(doctype, "expanded"); onExpandAll(); }} disabled={expanding}>
        <Icon name="chevrons-up-down" size={14} />{__("Expand all")}
      </button>
      <button class="btn sm" onclick={onCollapseAll} disabled={tree.expanded.size === 0}>
        <Icon name="chevrons-down-up" size={14} />{__("Collapse all")}
      </button>
    </div>
  {/if}
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
              <span class="toggle gap" aria-hidden="true"></span>
            {/if}
            <Icon name={row.expandable ? "folder" : "file"} size={14} />
            <a class="title" href={`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(row.node.id)}`}>{treeLabel(row.node, settings)}</a>
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
  .bar { display: flex; gap: 0.4rem; padding: 0 0.5rem 0.5rem; margin-bottom: 0.25rem; border-bottom: 1px solid var(--border); }
  .level { list-style: none; margin: 0; padding: 0; }
  .row { display: flex; align-items: center; gap: 0.4rem; padding: 0.3rem 0.5rem; padding-left: calc(0.5rem + var(--depth) * 1.25rem); border-radius: var(--radius); }
  .row:hover { background: var(--bg-subtle, rgba(0, 0, 0, 0.04)); }
  .toggle { width: 1.5rem; flex: none; }
  .toggle.gap { display: inline-block; }
  .title { font-weight: 500; }
  .count { margin-left: 0.1rem; }
  .add { margin-left: auto; }
  .more { padding-top: 0; }
  .empty { padding: 1rem; }
</style>
