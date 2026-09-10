<script lang="ts">
  // Form view generated from meta: sections/columns/tabs, controls, grids,
  // toolbar (save/submit/cancel/amend/delete), form-script buttons, sidebar.
  import { createForm, FormController, type Button } from "$lib/form.svelte";
  import { isLayout, type Field } from "$lib/meta";
  import Control from "$lib/controls/Control.svelte";
  import Grid from "$lib/controls/Grid.svelte";
  import Icon from "./Icon.svelte";
  import { __, boot } from "$lib/boot.svelte";
  import { showError, confirm, prompt, toast } from "$lib/ui.svelte";
  import { statusColor, timeAgo } from "$lib/format";
  import { onMount } from "svelte";
  import { subscribe } from "$lib/events";
  import { api } from "$lib/api";
  import { page } from "$app/state";
  import DocSidebar from "./DocSidebar.svelte";
  import { fieldsByRow } from "./form-layout";
  import { isSectionCollapsed, toggleSection } from "./section-state";
  import { getModifierKey, openShortcutsHelp } from "$lib/shortcuts.svelte";

  let { doctype, name }: { doctype: string; name: string } = $props();
  let frm = $state<FormController | null>(null);
  let error = $state("");
  let menuOpen = $state(false);
  let activeTab = $state(0);
  let collapsed = $state<Record<number, boolean>>({});
  let stale = $state(false);
  const modKey = $derived(getModifierKey());

  // O cleanup precisa ser registrado sincronamente: onMount ignora o que um
  // callback async *resolve*, então um corpo async vazaria a assinatura.
  onMount(() => {
    let alive = true;
    const off = subscribe("doc_update", (p: any) => {
      // nosso próprio save também ecoa aqui: só sinaliza mudança de outra pessoa, depois
      if (!alive || !frm || p.doctype !== doctype || p.name !== frm.doc.name) return;
      if (frm.saving) return;
      if (Date.now() - frm.loadedAt < 3000) return;
      if (p.user && boot.data?.user && p.user === boot.data.user) return;

      const pTime = p.modified ? new Date(p.modified).getTime() : 0;
      const docTime = frm.doc.modified ? new Date(frm.doc.modified).getTime() : 0;
      if (pTime > docTime + 1000) {
        stale = true;
      }
    });
    (async () => {
      try {
        const initial = (history.state as any)?.["sveltekit:states"]?.doc || (page.state as any)?.doc;
        const f = await createForm(doctype, name, initial);
        if (!alive) return; // saiu da página enquanto carregava
        frm = f;
        if (f.isNew) {
          for (const [k, v] of page.url.searchParams) if (f.field(k)) f.doc[k] = v;
        }
      } catch (e: any) { if (alive) error = e.message; }
    })();
    return () => { alive = false; off(); };
  });

  $effect(() => {
    // ao recarregar/atualizar o documento local, ele deixa de estar desatualizado
    if (frm?.doc.modified) {
      stale = false;
    }
  });

  // layout: tabs > sections > columns > fields
  interface Section { label?: string; collapsible?: boolean; columns: Field[][]; dependsOn?: string }
  interface Tab { label: string; sections: Section[] }
  const tabs = $derived.by((): Tab[] => {
    if (!frm) return [];
    const out: Tab[] = [{ label: __("Detalhes"), sections: [] }];
    const newSection = (f?: Field): Section => ({ label: f?.label, collapsible: f?.collapsible, columns: [[]], dependsOn: f?.dependsOn });
    let tab = out[0];
    let sec: Section | null = null;
    for (const f of frm.meta.doctype.fields) {
      const fx = frm.field(f.fieldname || "") || f;
      if (f.fieldtype === "Tab Break") { tab = { label: f.label || "", sections: [] }; out.push(tab); sec = null; continue; }
      if (f.fieldtype === "Section Break") { sec = newSection(fx); tab.sections.push(sec); continue; }
      if (!sec) { sec = newSection(); tab.sections.push(sec); }
      if (f.fieldtype === "Column Break") { sec.columns.push([]); continue; }
      sec.columns[sec.columns.length - 1].push(fx);
    }
    return out;
  });
  const status = $derived.by(() => {
    if (!frm) return "";
    const sf = frm.meta.doctype.fields.find((f) => ["status", "situacao"].includes(f.fieldname || ""));
    if (sf && frm.doc[sf.fieldname!]) return frm.doc[sf.fieldname!];
    if (frm.isNew) return __("Novo");
    if (frm.isSubmittable) return frm.docstatus === 2 ? __("Cancelado") : frm.docstatus === 1 ? __("Enviado") : __("Rascunho");
    return "";
  });
  const title = $derived(
    frm
      ? frm.isNew
        ? frm.doc.name?.trim() || __("Novo {0}", [frm.meta.doctype.label])
        : (frm.meta.doctype.titleField && frm.doc[frm.meta.doctype.titleField]) || frm.doc.name
      : ""
  );

  const canRename = $derived(!frm?.isNew && !!frm?.meta.doctype.allowRename && !!frm?.perm.write && frm?.docstatus === 0);
  const hasTitleField = $derived(!frm?.isNew && !!frm?.meta.doctype.titleField && !!frm?.perm.write && frm?.docstatus === 0);
  const canEditTitle = $derived(canRename || hasTitleField);
  const editTitleTooltip = $derived(
    hasTitleField
      ? __("Editar {0}", [frm?.field(frm?.meta.doctype.titleField!)?.label || __("título")])
      : __("Renomear {0}", [frm?.meta.doctype.label || doctype])
  );

  async function onEditTitle() {
    if (!frm) return;
    if (hasTitleField && frm.meta.doctype.titleField) {
      const tf = frm.meta.doctype.titleField;
      const fDef = frm.field(tf);
      const label = fDef?.label || __("Título");
      const v = await prompt(__("Editar {0}", [label]), [
        { fieldname: "title", fieldtype: "Data", label, reqd: true, default: frm.doc[tf] }
      ]);
      if (v && v.title !== undefined && v.title.trim() !== "" && v.title !== frm.doc[tf]) {
        frm.setValue(tf, v.title.trim());
      }
      return;
    }
    if (canRename) {
      await rename();
    }
  }

  async function remove() {
    if (!frm || !(await confirm(__("Apagar {0}?", [frm.doc.name]), __("Apagar")))) return;
    try { await frm.delete(); } catch (e) { showError(e); }
  }
  async function rename() {
    if (!frm) return;
    const label = frm.meta.doctype.label || frm.meta.doctype.name;
    const v = await prompt(__("Renomear {0}", [label]), [
      { fieldname: "name", fieldtype: "Data", label: __("Novo nome"), reqd: true, default: frm.doc.name }
    ]);
    if (!v || !v.name || v.name.trim() === frm.doc.name) return;
    try {
      const nn = await api.docMethod(doctype, frm.doc.name, "rename", { name: v.name.trim() });
      location.href = `/app/${encodeURIComponent(doctype)}/${encodeURIComponent(nn)}`;
    } catch (e) { showError(e); }
  }
  async function duplicate() {
    if (!frm) return;
    const copy = { ...frm.doc, name: undefined, __islocal: true, docstatus: 0, creation: undefined, modified: undefined, owner: undefined, amended_from: undefined };
    for (const f of frm.meta.doctype.fields) if (f.fieldtype === "Table") copy[f.fieldname!] = (copy[f.fieldname!] || []).map((r: any) => ({ ...r, name: undefined, parent: undefined }));
    frm.load(copy);
    history.replaceState(null, "", `/app/${encodeURIComponent(doctype)}/new`);
    await frm.runRefresh();
    toast(__("Cópia criada — salve para gravar"), { indicator: "blue" });
  }
  // dropdowns close on any click outside them (mouseleave used to need two clicks)
  function onPointerDown(e: PointerEvent) {
    if (!(menuOpen || openGroup)) return;
    if ((e.target as HTMLElement | null)?.closest(".dropdown")) return;
    menuOpen = false;
    openGroup = "";
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") { menuOpen = false; openGroup = ""; }
    if ((e.ctrlKey || e.metaKey) && e.key === "s") { e.preventDefault(); if (frm && !frm.readOnly) frm.save(); }
  }
  const groups = $derived.by(() => {
    const g = new Map<string, Button[]>();
    for (const b of frm?.buttons || []) { const k = b.group || ""; if (!g.has(k)) g.set(k, []); g.get(k)!.push(b); }
    return g;
  });
  let openGroup = $state("");
  const visibleSection = (s: Section) => !s.dependsOn || frm!.isFieldVisible({ fieldtype: "Section Break", dependsOn: s.dependsOn });
</script>

<svelte:window onkeydown={onKey} onpointerdown={onPointerDown} />

{#if error}
  <div class="page"><div class="card empty">{error}</div></div>
{:else if frm}
  <div class="page form-page">
    <div class="page-head">
      <div>
        <div class="small muted"><a href={`/app/${encodeURIComponent(doctype)}`}>{frm.meta.doctype.label}</a></div>
        <h1 style="display:flex;align-items:center;gap:8px">
          {#if canEditTitle}
            <button type="button" class="title-edit-btn" onclick={onEditTitle} title={editTitleTooltip}>
              <span>{title}</span>
              <span class="title-edit-icon" aria-hidden="true"><Icon name="edit-2" size={16} /></span>
            </button>
          {:else}
            <span>{title}</span>
          {/if}
          {#if status}<span class="indicator {statusColor(status)}">{status}</span>{/if}
          {#if frm.isDirty && !frm.isNew}<span class="indicator orange">{__("Não salvo")}</span>{/if}
        </h1>
      </div>
      <span class="spacer"></span>
      {#if stale}<button class="btn" onclick={() => { stale = false; frm?.reload(); }}><Icon name="refresh-cw" size={14} />{__("Alterado por outro usuário — recarregar")}</button>{/if}
      {#each frm.indicators as ind}<span class="indicator {ind.color}">{ind.label}</span>{/each}
      {#each [...groups] as [group, buttons]}
        {#if group}
          <div class="dropdown">
            <button class="btn" class:primary={buttons[0].primary} onclick={() => (openGroup = openGroup === group ? "" : group)}>{group} <Icon name="chevron-down" size={14} /></button>
            {#if openGroup === group}
              <div class="menu" role="menu" tabindex="-1">
                {#each buttons as b}<button onclick={() => { openGroup = ""; b.action(); }}>{b.label}</button>{/each}
              </div>
            {/if}
          </div>
        {:else}
          {#each buttons as b}<button class="btn" onclick={b.action}>{b.label}</button>{/each}
        {/if}
      {/each}
      {#if !frm.isNew}
        <div class="dropdown">
          <button class="btn icon" onclick={() => (menuOpen = !menuOpen)} aria-label="Menu"><Icon name="more-horizontal" /></button>
          {#if menuOpen}
            <div class="menu" role="menu" tabindex="-1">
              <button onclick={() => { menuOpen = false; frm?.reload(); }}>{__("Recarregar")}</button>
              {#if frm.perm.create}<button onclick={() => { menuOpen = false; duplicate(); }}>{__("Duplicar")}</button>{/if}
              {#if frm.meta.doctype.allowRename && frm.perm.write && frm.docstatus === 0}<button onclick={() => { menuOpen = false; rename(); }}>{__("Renomear")}</button>{/if}
              {#if frm.perm.delete && frm.docstatus !== 1}<button class="danger" style="color:var(--red)" onclick={() => { menuOpen = false; remove(); }}>{__("Apagar")}</button>{/if}
              <button onclick={() => { menuOpen = false; openShortcutsHelp(); }} style="display:flex;align-items:center;justify-content:space-between">
                <span>{__("Atalhos de teclado")}</span>
                <kbd class="kbd">?</kbd>
              </button>
            </div>
          {/if}
        </div>
      {/if}
      {#if frm.primaryAction}
        <button class="btn primary" onclick={frm.primaryAction.action}>{frm.primaryAction.label}</button>
      {:else if frm.isSubmittable}
        {#if frm.docstatus === 0}
          {#if frm.isDirty || frm.isNew}
            <button class="btn primary" disabled={frm.saving} onclick={() => frm?.save()} title="{__('Salvar')} ({modKey}+S)">{__("Salvar")}<kbd class="btn-kbd">{modKey}S</kbd></button>
          {:else if frm.perm.submit}
            <button class="btn primary" disabled={frm.saving} onclick={async () => (await confirm(__("Enviar permanentemente {0}?", [frm?.doc.name]), __("Enviar"))) && frm?.submit()}>{__("Enviar")}</button>
          {/if}
        {:else if frm.docstatus === 1}
          {#if frm.isDirty}<button class="btn primary" disabled={frm.saving} onclick={() => frm?.save()} title="{__('Atualizar')} ({modKey}+S)">{__("Atualizar")}<kbd class="btn-kbd">{modKey}S</kbd></button>
          {:else if frm.perm.cancel}<button class="btn" disabled={frm.saving} onclick={async () => (await confirm(__("Cancelar {0}?", [frm?.doc.name]), __("Cancelar"))) && frm?.cancel()}>{__("Cancelar")}</button>{/if}
        {:else if frm.perm.amend}
          <button class="btn primary" onclick={() => frm?.amend()}>{__("Emendar")}</button>
        {/if}
      {:else if frm.perm.write || (frm.isNew && frm.perm.create)}
        <button class="btn primary" disabled={frm.saving || (!frm.isDirty && !frm.isNew)} onclick={() => frm?.save()} title="{__('Salvar')} ({modKey}+S)">{__("Salvar")}<kbd class="btn-kbd">{modKey}S</kbd></button>
      {/if}
    </div>

    <div class="form-body">
      <div class="form-main">
        {#if tabs.length > 1}
          <div class="tabs">{#each tabs as t, i}<button class:active={activeTab === i} onclick={() => (activeTab = i)}>{t.label}</button>{/each}</div>
        {/if}
        {#each tabs[activeTab]?.sections || [] as sec, si}
          {#if visibleSection(sec) && sec.columns.some((c) => c.some((f) => frm!.isFieldVisible(f)))}
            <div class="form-section">
              {#if sec.label}
                {#if sec.collapsible}
                  <h3><button type="button" class="section-toggle" aria-expanded={!isSectionCollapsed(collapsed, si)} onclick={() => toggleSection(collapsed, si)}>{sec.label} <Icon name={isSectionCollapsed(collapsed, si) ? "chevron-right" : "chevron-down"} size={12} /></button></h3>
                {:else}
                  <h3>{sec.label}</h3>
                {/if}
              {/if}
              {#if !(sec.collapsible && isSectionCollapsed(collapsed, si))}
                {#each fieldsByRow(sec.columns) as row}
                  <div class="form-columns form-row" style="--cols:{sec.columns.length}">
                    {#each row as f}
                      <div class="form-cell">
                        {#if f && frm.isFieldVisible(f)}
                          {#if f.fieldtype === "Table"}
                            <Grid {frm} field={f} childMeta={frm.meta.children[f.options]} />
                          {:else if f.fieldtype === "HTML"}
                            <div class="field">{@html f.options || ""}</div>
                          {:else}
                            <Control field={f} value={frm.doc[f.fieldname!]} onchange={(v) => frm?.setValue(f.fieldname!, v)} doc={frm.doc}
                              readOnly={!frm.isFieldEditable(f)} mandatory={frm.isFieldMandatory(f)} error={frm.fieldErrors[f.fieldname!] || ""} query={frm.queries.get(f.fieldname!)} />
                          {/if}
                        {/if}
                      </div>
                    {/each}
                  </div>
                {/each}
              {/if}
            </div>
          {/if}
        {/each}
      </div>
      {#if !frm.isNew}
        <DocSidebar {frm} />
      {/if}
    </div>
  </div>
{:else}
  <div class="page muted">{__("Carregando...")}</div>
{/if}

<style>
  .form-body { display: flex; gap: 20px; align-items: flex-start; }
  .form-main { flex: 1; min-width: 0; }
  .tabs { display: flex; gap: 4px; border-bottom: 1px solid var(--border); margin-bottom: 14px; }
  .tabs button { border: 0; background: none; padding: 8px 12px; cursor: pointer; border-bottom: 2px solid transparent; color: var(--muted); }
  .tabs button.active { color: var(--primary); border-bottom-color: var(--primary); font-weight: 500; }
  .title-edit-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    background: none;
    border: none;
    padding: 2px 6px;
    margin: -2px -6px;
    border-radius: var(--radius, 6px);
    font: inherit;
    font-weight: inherit;
    color: inherit;
    cursor: pointer;
    text-align: left;
    transition: background-color 0.15s ease;
  }
  .title-edit-btn:hover {
    background: #f3f4f6;
  }
  .title-edit-icon {
    display: inline-flex;
    align-items: center;
    color: var(--muted);
    opacity: 0.5;
    transition: opacity 0.15s ease, color 0.15s ease;
  }
  .title-edit-btn:hover .title-edit-icon {
    opacity: 1;
    color: var(--primary);
  }
</style>
