<script lang="ts">
  // A card an app adds to the profile page with defineProfileSection: its
  // strings are shown as given, since the app wraps them in __() itself.
  import { __ } from "$lib/boot.svelte";
  import { toast } from "$lib/ui.svelte";
  import Control from "$lib/controls/Control.svelte";
  import type { ProfileSection } from "$lib/desk-sdk";
  import { missingRequired, sectionValues } from "./profile";
  import { onMount } from "svelte";

  let { section }: { section: ProfileSection } = $props();

  let loaded = $state(false);
  let data = $state<any>(null);
  let loadError = $state("");
  let values = $state<Record<string, any>>({});
  let error = $state(""), saving = $state(false);

  const fields = $derived((section.fields || []).filter((f) => f.fieldname));
  // a card whose data did not load shows why, and nothing that would need the data
  const info = $derived(!loadError && data != null && section.info ? section.info(data) || [] : []);

  onMount(load);

  async function load() {
    loadError = "";
    try {
      data = section.load ? await section.load() : {};
    } catch (e: any) {
      data = {};
      loadError = e?.message || String(e);
    }
    values = sectionValues(section.fields);
    loaded = true;
  }

  async function submit(e: Event) {
    e.preventDefault();
    if (!section.submit) return;
    // text controls commit on change, which fires on blur
    const active = document.activeElement as HTMLElement | null;
    if (active && active !== document.body) active.blur();
    const missing = missingRequired(section.fields, values);
    error = missing ? __("Fill in {0}", [missing]) : "";
    if (error) return;
    saving = true;
    try {
      await section.submit({ ...values }, data);
      if (section.successMessage) toast(section.successMessage, { indicator: "green" });
      await load();
    } catch (err: any) {
      error = err?.message || String(err);
    } finally {
      saving = false;
    }
  }
</script>

{#if loaded && data != null}
  <form class="card sect" onsubmit={submit}>
    <h2>{section.title}</h2>
    {#if loadError}
      <div class="err small">{loadError}</div>
    {:else}
      {#if info.length}
        <div class="grid">
          {#each info as line, i (i)}
            <div><span class="lbl">{line.label}</span><span>{line.value ?? "—"}</span></div>
          {/each}
        </div>
      {/if}
      {#if fields.length}
        <div class="row">
          {#each fields as f (f.fieldname)}
            <div class="fld">
              <Control field={f} value={values[f.fieldname!]} compact doc={values} onchange={(v: any) => (values[f.fieldname!] = v)} />
            </div>
          {/each}
        </div>
      {/if}
      {#if error}<div class="err small">{error}</div>{/if}
      {#if section.description}<p class="small muted">{section.description}</p>{/if}
      {#if section.submit}
        <button class="btn primary" disabled={saving}>{section.primaryLabel || __("Save")}</button>
      {/if}
    {/if}
  </form>
{/if}

<style>
  /* the look of the profile page's own cards */
  .sect { padding: 16px 20px; margin-bottom: 16px; }
  .sect h2 { font-size: 14px; margin: 0 0 12px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px 20px; margin-bottom: 12px; }
  .grid > div { display: flex; flex-direction: column; gap: 2px; }
  .lbl { font-size: 11px; text-transform: uppercase; letter-spacing: .05em; color: var(--muted); }
  .row { display: flex; flex-wrap: wrap; gap: 12px; align-items: flex-start; margin-bottom: 12px; }
  .fld { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 200px; }
  .err { color: var(--red); margin-bottom: 8px; }
</style>
