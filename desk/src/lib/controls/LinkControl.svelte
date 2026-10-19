<script lang="ts">
  import { seg, appPrefix } from "$lib/routes";
  // Link / Dynamic Link: typeahead over /api/search/link with a shortcut to open the target.
  import { api } from "$lib/api";
  import type { Field } from "$lib/meta";
  import { boot, __ } from "$lib/boot.svelte";
  import { getLinkTitle, setLinkTitle } from "$lib/titles.svelte";
  import { anchored } from "./floating";
  import { LinkSearch } from "./link-search.svelte";
  import { page } from "$app/state";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import { quickCreate } from "$lib/quick-create";
  import Icon from "$lib/components/Icon.svelte";
  import { virtualSubtitle } from "$lib/virtual";

  let { field, value, onchange, doc = {}, readOnly = false, query = undefined, error = "", id = "", search: searchFn = undefined }:
    { field: Field; value: any; onchange: (v: any) => void; doc?: any; readOnly?: boolean; query?: () => { filters?: any }; error?: string; id?: string;
      /** Replaces /api/search/link, answering `{ id, title }` rows — a portal's own search (OPS-10). It also hides the link to the desk. */
      search?: (txt: string) => Promise<{ id: string; title?: string }[]> } = $props();

  const target = $derived(field.fieldtype === "Dynamic Link" ? doc?.[field.options] : field.options);
  const currentTitle = $derived(target && value ? getLinkTitle(target, value) : "");
  const ls = new LinkSearch({
    target: () => target,
    fetch: (txt) => (searchFn ? searchFn(txt) : api.linkSearch(target, txt, query?.()?.filters, 20)),
    ownTitle: (o) => (searchFn ? String(o.title || o.id) : undefined),
  });
  let focused = $state(false);
  let inputEl: HTMLInputElement | null = $state(null);
  let listEl: HTMLDivElement | null = $state(null);

  // Keep the option the arrow keys reach inside the list's scroll.
  $effect(() => {
    const el = listEl?.children[ls.active] as HTMLElement | undefined;
    el?.scrollIntoView({ block: "nearest" });
  });

  $effect(() => {
    if (!focused) {
      ls.text = currentTitle || value || "";
    }
  });

  const titleField = $derived(target ? boot.data?.doctypes[target]?.titleField : undefined);
  const linkSubtitle = $derived(target ? boot.data?.doctypes[target]?.linkSubtitle : undefined);

  /** a union of DocTypes (DAT-07): each option says which one it comes from */
  const virtualTarget = $derived(!!(target && boot.data?.virtuals?.[target]));

  function getOptionSubtitle(o: any): string {
    if (!virtualTarget || searchFn) return ownSubtitle(o);
    return virtualSubtitle(o.id, ownSubtitle(o), boot.data?.doctypes);
  }

  function ownSubtitle(o: any): string {
    if (searchFn) return o.title && o.title !== o.id ? String(o.id) : "";
    if (linkSubtitle?.length) return linkSubtitle.map((f) => o[f]).filter((v) => v !== null && v !== undefined && v !== "").join(" · ");
    if (o._title) return o._title !== o.id ? String(o.id) : "";
    if (titleField && o[titleField]) {
      const others = Object.entries(o).filter(([k, v]) => k !== "id" && k !== titleField && v).map(([, v]) => v);
      return [o.id, ...others].join(" · ");
    }
    return Object.entries(o).filter(([k, v]) => k !== "id" && v).map(([, v]) => v).join(" · ");
  }

  function oninput(e: Event) {
    const txt = (e.target as HTMLInputElement).value;
    if (txt === "") onchange(null);
    ls.typed(txt, txt === "");
  }

  function pick(o: any) {
    const optTitle = ls.title(o);
    setLinkTitle(target, o.id, optTitle);
    onchange(o.id);
    ls.text = optTitle;
    ls.open = false;
  }

  function clear() {
    ls.cancel();
    ls.text = "";
    ls.options = [];
    ls.open = false;
    onchange(null);
    void ls.search("");
    inputEl?.focus();
  }

  function onblur() {
    focused = false;
    ls.blur(() => {
      const curTitle = currentTitle || value || "";
      if (ls.text === "" && value) {
        onchange(null);
      } else if (ls.text !== curTitle && ls.text !== value && ls.text !== "") {
        const exact = ls.options.find((o) => o.id === ls.text || ls.title(o) === ls.text || Object.values(o).includes(ls.text));
        if (exact) {
          pick(exact);
        } else {
          ls.text = curTitle;
        }
      } else {
        ls.text = curTitle;
      }
    });
  }

  // "+" creates the target in a dialog, for a user who may create it; the desk only, not a portal
  let canCreate = $state(false);
  $effect(() => {
    const dt = target;
    canCreate = false;
    if (!dt || readOnly || searchFn) return;
    getMeta(dt).then((m) => { if (dt === target) canCreate = !!m.permissions?.create; }).catch(() => {});
  });
  const showAdd = $derived(canCreate && !value && !!target && !readOnly && !searchFn);

  async function add() {
    ls.cancel();
    ls.open = false;
    const typed = ls.text;
    const created = await quickCreate(target, typed);
    if (!created) return;
    setLinkTitle(target, created.id, created.title);
    onchange(created.id);
    ls.text = created.title;
  }

  const label = $derived(target && boot.data?.doctypes[target]?.label);
  const wsPrefix = $derived(appPrefix(page.params?.workspace || getRememberedWorkspace()));
</script>

<div class="link-wrap" class:has-open={!!value && !!target} class:has-clear={!!value && !!target && !readOnly} class:has-add={showAdd}>
  <input bind:this={inputEl} {id} class="input" class:error={!!error} readonly={readOnly || !target} value={ls.text} placeholder={target ? "" : __("Choose the type first")} autocomplete="off"
    title={value ? `${ls.text}${ls.text !== value ? ` (${value})` : ""}` : ""}
    onfocus={() => { focused = true; if (!readOnly) ls.search(ls.text); }} {oninput} {onblur} onkeydown={(e) => ls.keydown(e, ls.options, pick)} data-fieldtype="Link" />
  {#if showAdd}
    <button type="button" class="add" aria-label={__("New {0}", [label || target])} title={__("New {0}", [label || target])} onmousedown={(e) => e.preventDefault()} onclick={add}><Icon name="plus" size={14} /></button>
  {/if}
  {#if value && target}
    {#if !readOnly}
      <button type="button" class="clear" aria-label={__("Clear {0} ({1})", [label || target, value])} title={__("Clear {0} ({1})", [label || target, value])} onmousedown={(e) => e.preventDefault()} onclick={clear}>×</button>
    {/if}
    {#if !searchFn}<a class="open" href={`${wsPrefix}/${seg(target)}/${encodeURIComponent(value)}`} title={__("Open {0} ({1})", [label || target, value])}>↗</a>{/if}
  {/if}
  {#if ls.open && !readOnly && ls.options.length}
    <div bind:this={listEl} class="options" role="listbox" use:anchored={{ anchor: inputEl, matchWidth: true, gap: 2, content: ls.options.length }}>
      {#each ls.options as o, i}
        <div role="option" tabindex="-1" aria-selected={i === ls.active} class:active={i === ls.active} onmousedown={() => pick(o)}>
          <div class="ttl">{ls.title(o)}</div>
          {#if getOptionSubtitle(o)}<div class="sub">{getOptionSubtitle(o)}</div>{/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
