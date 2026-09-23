<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  import { tick } from "svelte";
  import { goto } from "$app/navigation";
  import { api } from "$lib/api";
  import { boot, __ } from "$lib/boot.svelte";
  import { searchState, closeSearch } from "$lib/shortcuts.svelte";
  import Icon from "./Icon.svelte";
  import { getRememberedWorkspace, type WorkspaceItem } from "./sidebar-workspace";
  import { buildItems, moveIndex, SEARCH_MIN_CHARS, type PaletteItem, type SearchHit } from "./search-palette";

  let txt = $state("");
  let hits = $state<SearchHit[]>([]);
  let loading = $state(false);
  let failed = $state("");
  let index = $state(-1);
  let input = $state<HTMLInputElement>();
  let timer: ReturnType<typeof setTimeout> | undefined;
  // Only the answer to the latest request is shown: a slow reply to an older
  // prefix must not overwrite the results of what is typed now.
  let seq = 0;

  const items: PaletteItem[] = $derived(buildItems(txt, hits, {
    doctypes: boot.data?.doctypes || {},
    workspaces: (boot.data?.workspaces || []) as WorkspaceItem[],
    remembered: getRememberedWorkspace(),
  }));
  const tooShort = $derived(txt.trim().length < SEARCH_MIN_CHARS);

  $effect(() => {
    if (searchState.open) {
      txt = ""; hits = []; failed = ""; index = -1; loading = false;
      tick().then(() => input?.focus());
    } else {
      clearTimeout(timer);
    }
  });

  function onInput() {
    clearTimeout(timer);
    failed = "";
    index = -1;
    const current = txt.trim();
    const mine = ++seq;
    if (current.length < SEARCH_MIN_CHARS) { hits = []; loading = false; return; }
    loading = true;
    timer = setTimeout(async () => {
      try {
        const res = await api.globalSearch(current);
        if (mine === seq) hits = res || [];
      } catch (e: any) {
        if (mine === seq) { hits = []; failed = e?.message || String(e); }
      } finally {
        if (mine === seq) loading = false;
      }
    }, 200);
  }

  function open(item: PaletteItem | undefined) {
    if (!item) return;
    closeSearch();
    goto(item.href);
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); closeSearch(); }
    else if (e.key === "ArrowDown") { e.preventDefault(); index = moveIndex(index, 1, items.length); }
    else if (e.key === "ArrowUp") { e.preventDefault(); index = moveIndex(index, -1, items.length); }
    else if (e.key === "Enter") { e.preventDefault(); open(items[index >= 0 ? index : 0]); }
  }
</script>

{#if searchState.open}
  <div class="modal-bg" role="presentation" onclick={(e) => e.target === e.currentTarget && closeSearch()}>
    <div class="modal search-palette" use:focusTrap role="dialog" aria-modal="true" aria-label={__("Search")}>
      <div class="search-input">
        <Icon name="search" size={16} />
        <input
          bind:this={input}
          bind:value={txt}
          oninput={onInput}
          onkeydown={onKeydown}
          placeholder={__("Search documents and DocTypes…")}
          aria-label={__("Search")}
          role="combobox"
          aria-expanded={items.length > 0}
          aria-controls="search-results"
          aria-activedescendant={index >= 0 ? `search-item-${index}` : undefined}
          autocomplete="off"
        />
        {#if loading}<span class="muted small">…</span>{/if}
      </div>
      <ul id="search-results" class="search-results" role="listbox">
        {#each items as item, i (item.kind + item.doctype + item.id)}
          <li id={`search-item-${i}`} role="option" aria-selected={i === index}>
            <a href={item.href} class:active={i === index} onclick={(e) => { e.preventDefault(); open(item); }} onmousemove={() => (index = i)}>
              <Icon name={item.kind === "doctype" ? "list" : "notepad-text"} size={14} />
              <span class="title">{item.title}</span>
              <span class="muted small">{item.kind === "doctype" ? __("Go to list") : item.label}</span>
            </a>
          </li>
        {/each}
      </ul>
      {#if failed}
        <div class="search-note error small">{failed}</div>
      {:else if tooShort}
        <div class="search-note muted small">{__("Type at least {0} characters", [SEARCH_MIN_CHARS])}</div>
      {:else if !loading && items.length === 0}
        <div class="search-note muted small">{__("No results")}</div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .search-palette { max-width: 560px; overflow: hidden; }
  .search-input { display: flex; align-items: center; gap: 10px; padding: 12px 16px; border-bottom: 1px solid var(--border); color: var(--muted); }
  .search-input input { flex: 1; border: 0; outline: 0; font-size: 15px; background: none; color: var(--text); min-width: 0; }
  .search-results { list-style: none; margin: 0; padding: 4px; max-height: 60vh; overflow: auto; }
  .search-results:empty { display: none; }
  .search-results a { display: flex; align-items: center; gap: 10px; padding: 8px 10px; border-radius: 6px; color: var(--text); font-size: 13px; text-decoration: none; }
  .search-results a.active { background: #eff6ff; color: var(--primary); }
  .search-results .title { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .search-note { padding: 12px 16px; }
  .error { color: var(--danger, #dc2626); }
</style>
