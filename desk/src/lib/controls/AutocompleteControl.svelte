<script lang="ts">
  // Autocomplete: a text box that suggests values and accepts any other. The
  // suggestions are the field's `options`, read on every render so that
  // `frm.setDfProperty(field, "options", list)` changes them while the form is
  // open. They are never labels: what the list shows is what gets stored.
  // The list shares the Link typeahead's look (`.link-wrap` in app.css).
  import type { Field } from "$lib/meta";
  import { selectOptions } from "$lib/meta";
  import { anchored } from "./floating";
  import { committedValue, filterSuggestions, moveActive } from "./autocomplete-state";

  let { field, value, onchange, readOnly = false, error = "", id = "" }:
    { field: Field; value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string } = $props();

  let text = $state("");
  let open = $state(false);
  let active = $state(-1);
  let focused = $state(false);
  // until the user types, the list offers every suggestion rather than only
  // the ones matching the value already there
  let typed = $state(false);
  let inputEl: HTMLInputElement | null = $state(null);
  let listEl: HTMLDivElement | null = $state(null);

  const suggestions = $derived(filterSuggestions(selectOptions(field), typed ? text : ""));

  // the stored value shows whenever the user is not typing over it
  $effect(() => {
    if (!focused) text = value === null || value === undefined ? "" : String(value);
  });

  // Keep the suggestion the arrow keys reach inside the list's scroll.
  $effect(() => {
    const el = listEl?.children[active] as HTMLElement | undefined;
    el?.scrollIntoView?.({ block: "nearest" });
  });

  function commit(t: string) {
    const v = committedValue(t);
    text = v ?? "";
    if (v !== (value ?? null)) onchange(v);
  }

  function pick(s: string) {
    open = false;
    active = -1;
    commit(s);
  }

  function onfocus() {
    focused = true;
    typed = false;
    if (!readOnly) { active = -1; open = true; }
  }

  function oninput(e: Event) {
    text = (e.target as HTMLInputElement).value;
    typed = true;
    active = -1;
    open = true;
  }

  // Synchronous on purpose: the save shortcut and a dialog's primary button
  // blur the focused input and read the value right after.
  function onblur() {
    focused = false;
    open = false;
    active = -1;
    if (!readOnly) commit(text);
  }

  function onkeydown(e: KeyboardEvent) {
    if (readOnly) return;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      if (!open) { open = true; active = -1; }
      active = moveActive(active, e.key, suggestions.length);
      e.preventDefault();
    } else if (e.key === "Enter") {
      if (!open) return;
      if (active >= 0 && suggestions[active] !== undefined) pick(suggestions[active]);
      else { open = false; commit(text); }
      e.preventDefault();
    } else if (e.key === "Escape") {
      if (!open) return;
      open = false;
      active = -1;
      // the list closes; a dialog around it stays open
      e.preventDefault();
      e.stopPropagation();
    }
  }
</script>

<div class="link-wrap autocomplete">
  <input bind:this={inputEl} {id} class="input" class:error={!!error} type="text" readonly={readOnly} value={text} autocomplete="off"
    role="combobox" aria-autocomplete="list" aria-expanded={open && !readOnly && suggestions.length > 0}
    aria-controls={id ? `${id}-list` : undefined} aria-activedescendant={active >= 0 && id ? `${id}-opt-${active}` : undefined}
    maxlength={field.length || undefined} data-fieldname={field.fieldname} data-fieldtype="Autocomplete"
    {onfocus} {oninput} {onblur} {onkeydown} />
  {#if open && !readOnly && suggestions.length}
    <div bind:this={listEl} id={id ? `${id}-list` : undefined} class="options" role="listbox" use:anchored={{ anchor: inputEl, matchWidth: true, gap: 2, content: suggestions.length }}>
      {#each suggestions as s, i (s)}
        <!-- mousedown keeps the focus in the input, so blur does not commit the half-typed text first -->
        <div id={id ? `${id}-opt-${i}` : undefined} role="option" tabindex="-1" aria-selected={i === active} class:active={i === active}
          onmousedown={(e) => { e.preventDefault(); pick(s); }}>
          <div class="ttl">{s}</div>
        </div>
      {/each}
    </div>
  {/if}
</div>
