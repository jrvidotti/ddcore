<script lang="ts">
  // Link / Dynamic Link: typeahead over /api/search/link with a shortcut to open the target.
  import { api } from "$lib/api";
  import type { Field } from "$lib/meta";
  import { boot } from "$lib/boot.svelte";
  import { getLinkTitle, setLinkTitle } from "$lib/titles.svelte";

  let { field, value, onchange, doc = {}, readOnly = false, query = undefined, error = "", id = "" }:
    { field: Field; value: any; onchange: (v: any) => void; doc?: any; readOnly?: boolean; query?: () => { filters?: any }; error?: string; id?: string } = $props();

  const target = $derived(field.fieldtype === "Dynamic Link" ? doc?.[field.options] : field.options);
  const currentTitle = $derived(target && value ? getLinkTitle(target, value) : "");
  let text = $state("");
  let open = $state(false);
  let options = $state<any[]>([]);
  let active = $state(0);
  let timer: any;
  let focused = $state(false);

  $effect(() => {
    if (!focused) {
      text = currentTitle || value || "";
    }
  });

  const titleField = $derived(target ? boot.data?.doctypes[target]?.titleField : undefined);

  function getOptionTitle(o: any): string {
    if (titleField && o[titleField]) return String(o[titleField]);
    return o.name;
  }

  function getOptionSubtitle(o: any): string {
    if (titleField && o[titleField]) {
      const others = Object.entries(o).filter(([k, v]) => k !== "name" && k !== titleField && v).map(([, v]) => v);
      return [o.name, ...others].join(" · ");
    }
    return Object.entries(o).filter(([k, v]) => k !== "name" && v).map(([, v]) => v).join(" · ");
  }

  async function search(txt: string) {
    if (!target) { options = []; return; }
    try {
      const filters = query?.()?.filters;
      options = await api.linkSearch(target, txt, filters, 20);
      for (const o of options) {
        setLinkTitle(target, o.name, getOptionTitle(o));
      }
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
    const optTitle = getOptionTitle(o);
    setLinkTitle(target, o.name, optTitle);
    onchange(o.name);
    text = optTitle;
    open = false;
  }

  function onblur() {
    focused = false;
    setTimeout(() => {
      open = false;
      const curTitle = currentTitle || value || "";
      if (text === "" && value) {
        onchange(null);
      } else if (text !== curTitle && text !== value && text !== "") {
        const exact = options.find((o) => o.name === text || getOptionTitle(o) === text || Object.values(o).includes(text));
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
    else if (e.key === "Escape") open = false;
  }

  const label = $derived(target && boot.data?.doctypes[target]?.label);
</script>

<div class="link-wrap">
  <input {id} class="input" class:error={!!error} readonly={readOnly || !target} value={text} placeholder={target ? "" : "Escolha o tipo antes"} autocomplete="off"
    title={value ? `${text}${text !== value ? ` (${value})` : ""}` : ""}
    onfocus={() => { focused = true; if (!readOnly) search(text); }} {oninput} {onblur} {onkeydown} data-fieldtype="Link" />
  {#if value && target}
    <a class="open" href={`/app/${encodeURIComponent(target)}/${encodeURIComponent(value)}`} title="Abrir {label || target}{value ? ` (${value})` : ""}">↗</a>
  {/if}
  {#if open && !readOnly && options.length}
    <div class="options" role="listbox">
      {#each options as o, i}
        <div role="option" tabindex="-1" aria-selected={i === active} class:active={i === active} onmousedown={() => pick(o)}>
          <div>{getOptionTitle(o)}</div>
          {#if getOptionSubtitle(o)}<div class="sub">{getOptionSubtitle(o)}</div>{/if}
        </div>
      {/each}
    </div>
  {/if}
</div>
