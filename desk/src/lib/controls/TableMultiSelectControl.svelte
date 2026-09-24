<script lang="ts">
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

  let text = $state("");
  let open = $state(false);
  let options = $state<any[]>([]);
  let active = $state(0);
  let timer: any;
  let searchVersion = 0;
  let inputEl: HTMLInputElement | null = $state(null);
  let wrapEl: HTMLDivElement | null = $state(null);

  const titleField = $derived(target ? boot.data?.doctypes[target]?.titleField : undefined);
  function optionTitle(o: any): string {
    if (o._title) return String(o._title);
    if (titleField && o[titleField]) return String(o[titleField]);
    return o.id;
  }
  const shown = $derived(options.filter((o) => !values.includes(o.id)));

  async function search(txt: string) {
    if (!target) return;
    const version = ++searchVersion;
    try {
      // one more than a page, so a page of choices already made still leaves some
      const results = await api.linkSearch(target, txt, query?.()?.filters, 20 + values.length);
      if (version !== searchVersion) return;
      for (const o of results) setLinkTitle(target, o.id, optionTitle(o));
      options = results;
      active = 0;
      open = true;
    } catch { options = []; }
  }

  function oninput(e: Event) {
    text = (e.target as HTMLInputElement).value;
    clearTimeout(timer);
    timer = setTimeout(() => search(text), 150);
  }

  function pick(o: any) {
    if (!link || !target || !child) return;
    setLinkTitle(target, o.id, optionTitle(o));
    onchange(withValue(value, link.fieldname!, o.id, child.name));
    text = "";
    void search("");
    inputEl?.focus();
  }

  function remove(v: string) {
    if (!link) return;
    onchange(withoutValue(value, link.fieldname!, v));
  }

  function onblur() {
    clearTimeout(timer);
    searchVersion++;
    setTimeout(() => { open = false; text = ""; }, 150);
  }

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "Backspace" && text === "" && values.length) {
      remove(values[values.length - 1]);
      e.preventDefault();
      return;
    }
    if (!open) return;
    if (e.key === "ArrowDown") { active = Math.min(active + 1, shown.length - 1); e.preventDefault(); }
    else if (e.key === "ArrowUp") { active = Math.max(active - 1, 0); e.preventDefault(); }
    else if (e.key === "Enter") { if (shown[active]) pick(shown[active]); e.preventDefault(); }
    else if (e.key === "Escape") { open = false; e.preventDefault(); }
  }

  const label = $derived(target && boot.data?.doctypes[target]?.label);
  const workspace = $derived(page.params?.workspace || getRememberedWorkspace() || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");
</script>

<!-- svelte-ignore a11y_click_events_have_key_events, a11y_no_static_element_interactions -->
<div bind:this={wrapEl} class="input ms-wrap" class:error={!!error} class:readonly={readOnly} data-fieldtype="Table MultiSelect"
  onclick={() => { if (!readOnly) inputEl?.focus(); }}>
  {#each values as v (v)}
    <span class="ms-pill">
      {#if readOnly && target}
        <a href={`${wsPrefix}/${encodeURIComponent(target)}/${encodeURIComponent(v)}`} title={v}>{getLinkTitle(target, v)}</a>
      {:else}
        <span title={v}>{target ? getLinkTitle(target, v) : v}</span>
        <button type="button" class="ms-remove" aria-label={__("Remove {0}", [target ? getLinkTitle(target, v) : v])}
          onmousedown={(e) => e.preventDefault()} onclick={(e) => { e.stopPropagation(); remove(v); }}>×</button>
      {/if}
    </span>
  {/each}
  {#if !readOnly}
    <input bind:this={inputEl} {id} class="ms-input" value={text} autocomplete="off" disabled={!target}
      placeholder={values.length ? "" : label ? __("Add {0}", [label]) : ""}
      onfocus={() => search(text)} {oninput} {onblur} {onkeydown} />
  {/if}
  {#if open && !readOnly && shown.length}
    <div class="ms-options" role="listbox" use:anchored={{ anchor: wrapEl, matchWidth: true, gap: 2, content: shown.length }}>
      {#each shown as o, i (o.id)}
        <div role="option" tabindex="-1" aria-selected={i === active} class:active={i === active} onmousedown={() => pick(o)}>{optionTitle(o)}</div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .ms-wrap { display: flex; flex-wrap: wrap; align-items: center; gap: 4px; padding: 3px 6px; height: auto; cursor: text; }
  .ms-wrap.readonly { cursor: default; background: #f9fafb; }
  .ms-wrap:focus-within { border-color: var(--primary); box-shadow: 0 0 0 2px rgba(37,99,235,.15); }
  .ms-pill { display: inline-flex; align-items: center; gap: 4px; max-width: 100%; padding: 1px 4px 1px 8px; border-radius: 999px; background: #eef0f3; color: #374151; font-size: 12px; line-height: 20px; }
  .ms-wrap.readonly .ms-pill { padding-right: 8px; }
  .ms-pill > span, .ms-pill > a { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ms-pill > a { color: inherit; }
  .ms-remove { width: 16px; height: 16px; padding: 0; border: 0; border-radius: 50%; display: grid; place-items: center; font: inherit; line-height: 1; color: var(--muted); background: transparent; cursor: pointer; }
  .ms-remove:hover { color: var(--text); background: #d9dde3; }
  .ms-remove:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  .ms-input { flex: 1; min-width: 80px; border: 0; outline: none; background: transparent; padding: 2px; font: inherit; }
  .ms-options { position: fixed; background: #fff; border: 1px solid var(--border); border-radius: 6px; box-shadow: 0 8px 24px rgba(0,0,0,.12); z-index: 80; max-height: 260px; overflow: auto; }
  .ms-options > div { padding: 6px 10px; cursor: pointer; font-size: 13px; line-height: 1.25; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .ms-options > div.active, .ms-options > div:hover { background: #eff6ff; }
</style>
