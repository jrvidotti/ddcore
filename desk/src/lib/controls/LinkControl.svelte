<script lang="ts">
  // Link / Dynamic Link: typeahead over /api/search/link with a shortcut to open the target.
  import { api } from "$lib/api";
  import type { Field } from "$lib/meta";
  import { boot, __ } from "$lib/boot.svelte";
  import { getLinkTitle, setLinkTitle } from "$lib/titles.svelte";
  import { anchored } from "./floating";
  import { page } from "$app/state";
  import { getRememberedWorkspace } from "$lib/components/sidebar-workspace";
  import { getMeta } from "$lib/meta";
  import { quickCreate } from "$lib/quick-create";
  import Icon from "$lib/components/Icon.svelte";

  let { field, value, onchange, doc = {}, readOnly = false, query = undefined, error = "", id = "", search: searchFn = undefined }:
    { field: Field; value: any; onchange: (v: any) => void; doc?: any; readOnly?: boolean; query?: () => { filters?: any }; error?: string; id?: string;
      /** Replaces /api/search/link, answering `{ id, title }` rows — a portal's own search (OPS-10). It also hides the link to the desk. */
      search?: (txt: string) => Promise<{ id: string; title?: string }[]> } = $props();

  const target = $derived(field.fieldtype === "Dynamic Link" ? doc?.[field.options] : field.options);
  const currentTitle = $derived(target && value ? getLinkTitle(target, value) : "");
  let text = $state("");
  let open = $state(false);
  let options = $state<any[]>([]);
  let active = $state(0);
  let timer: any;
  let searchVersion = 0;
  let focused = $state(false);
  let inputEl: HTMLInputElement | null = $state(null);
  let listEl: HTMLDivElement | null = $state(null);

  // Keep the option the arrow keys reach inside the list's scroll.
  $effect(() => {
    const el = listEl?.children[active] as HTMLElement | undefined;
    el?.scrollIntoView({ block: "nearest" });
  });

  $effect(() => {
    if (!focused) {
      text = currentTitle || value || "";
    }
  });

  const titleField = $derived(target ? boot.data?.doctypes[target]?.titleField : undefined);
  const linkSubtitle = $derived(target ? boot.data?.doctypes[target]?.linkSubtitle : undefined);

  function getOptionTitle(o: any): string {
    if (searchFn) return String(o.title || o.id);
    // a translateId target: the server sends the translated id
    if (o._title) return String(o._title);
    if (titleField && o[titleField]) return String(o[titleField]);
    return o.id;
  }

  function getOptionSubtitle(o: any): string {
    if (searchFn) return o.title && o.title !== o.id ? String(o.id) : "";
    if (linkSubtitle?.length) return linkSubtitle.map((f) => o[f]).filter((v) => v !== null && v !== undefined && v !== "").join(" · ");
    if (o._title) return o._title !== o.id ? String(o.id) : "";
    if (titleField && o[titleField]) {
      const others = Object.entries(o).filter(([k, v]) => k !== "id" && k !== titleField && v).map(([, v]) => v);
      return [o.id, ...others].join(" · ");
    }
    return Object.entries(o).filter(([k, v]) => k !== "id" && v).map(([, v]) => v).join(" · ");
  }

  async function search(txt: string) {
    if (!target) { options = []; return; }
    const version = ++searchVersion;
    try {
      const filters = query?.()?.filters;
      const results = searchFn ? await searchFn(txt) : await api.linkSearch(target, txt, filters, 20);
      if (version !== searchVersion) return;
      options = results;
      for (const o of results) {
        setLinkTitle(target, o.id, getOptionTitle(o));
      }
      active = 0;
      open = true;
    } catch { options = []; }
  }

  function oninput(e: Event) {
    text = (e.target as HTMLInputElement).value;
    clearTimeout(timer);
    if (text === "") {
      onchange(null);
      void search("");
      return;
    }
    timer = setTimeout(() => search(text), 150);
  }

  function pick(o: any) {
    const optTitle = getOptionTitle(o);
    setLinkTitle(target, o.id, optTitle);
    onchange(o.id);
    text = optTitle;
    open = false;
  }

  function clear() {
    clearTimeout(timer);
    searchVersion++;
    text = "";
    options = [];
    open = false;
    onchange(null);
    void search("");
    inputEl?.focus();
  }

  function onblur() {
    focused = false;
    clearTimeout(timer);
    searchVersion++;
    setTimeout(() => {
      open = false;
      const curTitle = currentTitle || value || "";
      if (text === "" && value) {
        onchange(null);
      } else if (text !== curTitle && text !== value && text !== "") {
        const exact = options.find((o) => o.id === text || getOptionTitle(o) === text || Object.values(o).includes(text));
        if (exact) {
          pick(exact);
        } else {
          text = curTitle;
        }
      } else {
        text = curTitle;
      }
    }, 150);
  }

  function onkeydown(e: KeyboardEvent) {
    if (!open) return;
    if (e.key === "ArrowDown") { active = Math.min(active + 1, options.length - 1); e.preventDefault(); }
    else if (e.key === "ArrowUp") { active = Math.max(active - 1, 0); e.preventDefault(); }
    else if (e.key === "Enter") { if (options[active]) pick(options[active]); e.preventDefault(); }
    else if (e.key === "Escape") { open = false; e.preventDefault(); }
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
    clearTimeout(timer);
    searchVersion++;
    open = false;
    const typed = text;
    const created = await quickCreate(target, typed);
    if (!created) return;
    setLinkTitle(target, created.id, created.title);
    onchange(created.id);
    text = created.title;
  }

  const label = $derived(target && boot.data?.doctypes[target]?.label);
  const workspace = $derived(page.params?.workspace || getRememberedWorkspace() || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");
</script>

<div class="link-wrap" class:has-open={!!value && !!target} class:has-clear={!!value && !!target && !readOnly} class:has-add={showAdd}>
  <input bind:this={inputEl} {id} class="input" class:error={!!error} readonly={readOnly || !target} value={text} placeholder={target ? "" : "Escolha o tipo antes"} autocomplete="off"
    title={value ? `${text}${text !== value ? ` (${value})` : ""}` : ""}
    onfocus={() => { focused = true; if (!readOnly) search(text); }} {oninput} {onblur} {onkeydown} data-fieldtype="Link" />
  {#if showAdd}
    <button type="button" class="add" aria-label={__("New {0}", [label || target])} title={__("New {0}", [label || target])} onmousedown={(e) => e.preventDefault()} onclick={add}><Icon name="plus" size={14} /></button>
  {/if}
  {#if value && target}
    {#if !readOnly}
      <button type="button" class="clear" aria-label="Limpar {label || target} ({value})" title="Limpar {label || target} ({value})" onmousedown={(e) => e.preventDefault()} onclick={clear}>×</button>
    {/if}
    {#if !searchFn}<a class="open" href={`${wsPrefix}/${encodeURIComponent(target)}/${encodeURIComponent(value)}`} title="Abrir {label || target}{value ? ` (${value})` : ""}">↗</a>{/if}
  {/if}
  {#if open && !readOnly && options.length}
    <div bind:this={listEl} class="options" role="listbox" use:anchored={{ anchor: inputEl, matchWidth: true, gap: 2, content: options.length }}>
      {#each options as o, i}
        <div role="option" tabindex="-1" aria-selected={i === active} class:active={i === active} onmousedown={() => pick(o)}>
          <div class="ttl">{getOptionTitle(o)}</div>
          {#if getOptionSubtitle(o)}<div class="sub">{getOptionSubtitle(o)}</div>{/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
