<script lang="ts">
  import { seg } from "$lib/routes";
  // Table MultiSelect: the child rows shown as pills, one per chosen value, with
  // a typeahead over the child's Link target to add more.
  import { api } from "$lib/api";
  import type { DocTypeMeta, Field } from "$lib/meta";
  import { getMeta } from "$lib/meta";
  import { boot, __ } from "$lib/boot.svelte";
  import { getLinkTitle, setLinkTitle } from "$lib/titles.svelte";
  import { page } from "$app/state";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";
  import { anchored } from "./floating";
  import { LinkSearch } from "./link-search.svelte";
  import { multiSelectLinkField, multiSelectValues, withValue, withoutValue } from "./multiselect-state";

  let { field, value, onchange, doc = {}, readOnly = false, query = undefined, error = "", id = "", childMeta = undefined }:
    { field: Field; value: any; onchange: (v: any) => void; doc?: any; readOnly?: boolean; query?: () => { filters?: any }; error?: string; id?: string;
      /** The child DocType; read from the parent's meta when left out. */
      childMeta?: DocTypeMeta } = $props();

  // the parent's meta carries its children, and judges their fields by its own levels
  let loadedChild = $state<DocTypeMeta | undefined>(undefined);
  $effect(() => {
    const parent = doc?.doctype;
    if (childMeta || !parent || !field.options) return;
    getMeta(parent).then((m) => { loadedChild = m.children?.[field.options]; }).catch(() => {});
  });
  const child = $derived(childMeta || loadedChild);
  const link = $derived(multiSelectLinkField(child));
  const target = $derived(link?.options as string | undefined);
  const values = $derived(link ? multiSelectValues(value, link.fieldname!) : []);

  // one more than a page, so a page of choices already made still leaves some
  const ls = new LinkSearch({ target: () => target, fetch: (txt) => api.linkSearch(target!, txt, query?.()?.filters, 20 + values.length) });
  let inputEl: HTMLInputElement | null = $state(null);
  let wrapEl: HTMLDivElement | null = $state(null);

  const shown = $derived(ls.options.filter((o) => !values.includes(o.id)));

  function oninput(e: Event) { ls.typed((e.target as HTMLInputElement).value); }

  // the list stays open after a pick, so several values can be chosen in a row;
  // the option's mousedown keeps the focus in the input for that
  function pick(o: any) {
    if (!link || !target || !child) return;
    setLinkTitle(target, o.id, ls.title(o));
    onchange(withValue(value, link.fieldname!, o.id, child.name));
    ls.text = "";
    void ls.search("");
    inputEl?.focus();
  }

  function remove(v: string) {
    if (!link) return;
    onchange(withoutValue(value, link.fieldname!, v));
  }

  function onblur() { ls.blur(() => (ls.text = "")); }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Backspace" && ls.text === "" && values.length) {
      remove(values[values.length - 1]);
      e.preventDefault();
      return;
    }
    ls.keydown(e, shown, pick);
  }

  const label = $derived(target && boot.data?.doctypes[target]?.label);
  const workspace = $derived(page.params?.workspace || getRememberedWorkspace() || "");
  const wsPrefix = $derived(workspace ? `/app/${seg(workspace)}` : "/app");
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div bind:this={wrapEl} class="input ms-wrap" class:error={!!error} class:readonly={readOnly} data-fieldtype="Table MultiSelect"
  onclick={() => { if (!readOnly) inputEl?.focus(); }}>
  {#each values as v (v)}
    <span class="ms-pill">
      {#if readOnly && target}
        <a href={`${wsPrefix}/${seg(target)}/${encodeURIComponent(v)}`} title={v}>{getLinkTitle(target, v)}</a>
      {:else}
        <span title={v}>{target ? getLinkTitle(target, v) : v}</span>
        <button type="button" class="ms-remove" aria-label={__("Remove {0}", [target ? getLinkTitle(target, v) : v])}
          onmousedown={(e) => e.preventDefault()} onclick={(e) => { e.stopPropagation(); remove(v); }}>×</button>
      {/if}
    </span>
  {/each}
  {#if !readOnly}
    <input bind:this={inputEl} {id} class="ms-input" value={ls.text} autocomplete="off" disabled={!target}
      placeholder={values.length ? "" : label ? __("Add {0}", [label]) : ""}
      onfocus={() => ls.search(ls.text)} {oninput} {onblur} {onkeydown} />
  {/if}
  {#if ls.open && !readOnly && shown.length}
    <div class="ms-options" role="listbox" use:anchored={{ anchor: wrapEl, matchWidth: true, gap: 2, content: shown.length }}>
      {#each shown as o, i (o.id)}
        <div role="option" tabindex="-1" aria-selected={i === ls.active} class:active={i === ls.active} onmousedown={(e) => { e.preventDefault(); pick(o); }}>{ls.title(o)}</div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .ms-wrap { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; padding: 3px 6px; height: auto; cursor: text; }
  .ms-wrap.readonly { cursor: default; background: var(--surface-alt); }
  .ms-wrap:focus-within { border-color: var(--primary); box-shadow: 0 0 0 2px var(--focus-ring); }
  .ms-pill { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; padding: 1px 4px 1px 8px; border-radius: 999px; background: var(--hover-strong); color: var(--text); font-size: 12px; line-height: 20px; }
  .ms-wrap.readonly .ms-pill { padding-right: 8px; }
  .ms-pill > span, .ms-pill > a { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ms-pill > a { color: inherit; }
  .ms-remove { width: 16px; height: 16px; padding: 0; border: 0; border-radius: 50%; display: grid; place-items: center; font: inherit; line-height: 1; color: var(--muted); background: transparent; cursor: pointer; }
  .ms-remove:hover { color: var(--text); background: color-mix(in srgb, var(--text) 18%, var(--surface)); }
  .ms-remove:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  .ms-input { flex: 1; min-width: 80px; border: 0; outline: none; background: transparent; padding: 2px; font: inherit; }
  .ms-options { position: fixed; background: var(--surface); border: 1px solid var(--border); border-radius: 6px; box-shadow: var(--shadow-pop); z-index: 80; max-height: 260px; overflow: auto; }
  .ms-options > div { padding: 6px 10px; cursor: pointer; font-size: 13px; line-height: 1.25; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ms-options > div.active, .ms-options > div:hover { background: var(--active); }
</style>
