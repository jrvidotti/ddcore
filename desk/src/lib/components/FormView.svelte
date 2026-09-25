<script lang="ts">
  // Form view generated from meta: sections/tabs, controls, grids,
  // toolbar (save/submit/cancel/amend/delete), form-script buttons, sidebar.
  import { createForm, FormController, type Button } from "$lib/form.svelte";
  import { isLayout, isTableType, selectLabels, selectOptions, type Field } from "$lib/meta";
  import { treeParentQuery } from "./views/tree-state";
  import Control from "$lib/controls/Control.svelte";
  import Grid from "$lib/controls/Grid.svelte";
  import Icon from "./Icon.svelte";
  import Spinner from "./Spinner.svelte";
  import { __, boot } from "$lib/boot.svelte";
  import { showError, confirm, dialog, prompt, toast, escapeHtml } from "$lib/ui.svelte";
  import { statusColor, timeAgo } from "$lib/format";
  import { onMount } from "svelte";
  import { subscribe } from "$lib/events";
  import { api } from "$lib/api";
  import { page } from "$app/state";
  import { beforeNavigate, goto } from "$app/navigation";
  import { clearDraft, draftDecision, draftKey, localDrafts, pruneDrafts, readDraft, writeDraft } from "$lib/drafts";
  import DocSidebar from "./DocSidebar.svelte";
  import { cellWidthClass, LINE_SLOTS, packLines } from "./form-layout";
  import { isSectionCollapsed, toggleSection } from "./section-state";
  import { resolveActiveTab, tabToSearchParams } from "./form-tabs";
  import { workspaceFor } from "./search-palette";
  import { getRememberedWorkspace, type WorkspaceItem } from "./sidebar-workspace";
  import { commitFocusedEdit, getModifierKey, openShortcutsHelp } from "$lib/shortcuts.svelte";

  let { doctype, id, basePath: ownBase = "" }: {
    doctype: string; id: string;
    /** a page with its own routes for the doctype (the To-Do page: /app/todo); else the workspace's */
    basePath?: string;
  } = $props();
  let frm = $state<FormController | null>(null);
  let error = $state("");
  let menuOpen = $state(false);
  let workflowMenuOpen = $state(false);
  let activeTab = $state(0);
  let collapsed = $state<Record<number, boolean>>({});
  let stale = $state(false);
  const modKey = $derived(getModifierKey());

  // what the user typed and has not saved, kept in the browser
  const drafts = localDrafts();
  /** set by duplicate(), which turns the open record into an unsaved copy */
  let duplicated = $state(false);
  /** the record the draft belongs to */
  const draftRecord = () => (duplicated ? "new" : id);
  const currentDraftKey = () => draftKey(boot.data?.user, doctype, draftRecord());
  /** set when the user has agreed to leave, so the guard lets the navigation through */
  let leaving = false;

  const workspace = $derived(page.params.workspace || "");
  const wsPrefix = $derived(`/app/${encodeURIComponent(workspace || workspaceFor(doctype, (boot.data?.workspaces || []) as WorkspaceItem[], boot.data?.doctypes, getRememberedWorkspace()))}`);
  /** the list, and the records under it */
  const basePath = $derived(ownBase || `${wsPrefix}/${encodeURIComponent(doctype)}`);
  /** set once the record is deleted: it must never get a draft again */
  let gone = false;
  /** the draft is only kept in step after the one it may have recovered is in */
  let recovered = $state(false);

  // Cleanup must be registered synchronously: onMount ignores what an
  // async callback *resolves*, so an async body would leak the subscription.
  onMount(() => {
    let alive = true;
    const off = subscribe("doc_update", (p: any) => {
      // our own save also echoes here: only signal someone else's change
      if (!alive || !frm || p.doctype !== doctype || p.id !== frm.doc.id) return;
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
        const f = await createForm(doctype, id, initial, ownBase);
        if (!alive) return; // navigated away while loading
        frm = f;
        if (f.isNew && !f.isSingle) {
          for (const [k, v] of page.url.searchParams) if (f.field(k)) f.doc[k] = v;
        }
        await recoverDraft(f);
      } catch (e: any) { if (alive) error = e.message; }
    })();
    return () => { alive = false; off(); flushDraft(); };
  });

  $effect(() => {
    // when reloading/updating local doc, it is no longer stale
    if (frm?.doc.modified) {
      stale = false;
    }
  });

  /** Puts back what the user had typed and not saved the last time they were here. */
  async function recoverDraft(f: FormController) {
    try {
      pruneDrafts(drafts);
      const key = currentDraftKey();
      const draft = readDraft(drafts, key);
      const decision = draftDecision(draft, f.doc);
      if (decision === "none") { if (draft) clearDraft(drafts, key); return; }
      if (decision === "conflict" && !(await confirm(
        __("This record changed after your unsaved edits. Keep your edits?"), __("Unsaved changes")))) {
        clearDraft(drafts, key);
        return;
      }
      f.applyDraft(draft!.doc);
      await f.runRefresh();
      if (decision === "restore") toast(__("Unsaved changes recovered"), { indicator: "blue" });
    } finally {
      recovered = true;
    }
  }

  function persist(f: FormController) {
    const doc = $state.snapshot(f.doc) as any;
    writeDraft(drafts, currentDraftKey(), { doctype, id: draftRecord(), doc, savedAt: Date.now(), modified: doc.modified ?? null });
  }

  /**
   * Writes the draft right now, for the moments that do not wait: closing the
   * tab, or leaving the form before the debounce below has fired. The focused
   * field has not committed its value yet either, so flush that first.
   */
  function flushDraft() {
    commitFocusedEdit(document.activeElement);
    const f = frm;
    if (!f || gone || f.saving || !f.isDirty) return;
    persist(f);
  }

  // keep the draft in step with the form, and drop it the moment there is
  // nothing unsaved left — which is what saving the document amounts to
  $effect(() => {
    const f = frm;
    if (!f || f.loading || !recovered) return;
    const dirty = f.isDirty; // reads the whole document, so every edit re-runs this
    if (f.saving) return;
    const key = currentDraftKey();
    if (!dirty) { clearDraft(drafts, key); return; }
    const t = setTimeout(() => persist(f), 400);
    return () => clearTimeout(t);
  });

  // leaving a form with unsaved changes asks first — the draft is the safety
  // net, the question is what the user meant to do
  beforeNavigate((nav) => {
    if (leaving) return;
    flushDraft(); // whatever happens next, what was typed is kept
    if (!frm?.isDirty || frm.saving) return;
    if (nav.type === "leave") { nav.cancel(); return; } // the browser asks in its own dialog
    const to = nav.to?.url;
    if (!to) return;
    nav.cancel();
    const go = async (discard: boolean) => {
      h.hide();
      // a clean form leaves nothing for the unmount to write back as a draft
      if (discard && frm) { await frm.discardChanges(); clearDraft(drafts, currentDraftKey()); }
      leaving = true;
      await goto(to);
    };
    const h = dialog({
      title: __("Unsaved changes"), size: "sm",
      message: __("Leave without saving? Your changes are kept as a draft."),
      primaryLabel: __("Yes"), secondaryLabel: __("No"),
      primaryAction: () => go(false),
      dangerLabel: __("Discard changes"), dangerShortcut: true,
      dangerAction: () => go(true),
    });
    h.show();
  });

  async function discard() {
    if (!frm || !(await confirm(__("Discard your unsaved changes?"), __("Discard changes"), { destructive: true }))) return;
    clearDraft(drafts, currentDraftKey());
    await frm.discardChanges();
    toast(__("Changes discarded"), { indicator: "blue", timeout: 2000 });
  }

  // layout: tabs > sections > fields
  interface Section { label?: string; collapsible?: boolean; fields: Field[]; dependsOn?: string }
  interface Tab { label: string; sections: Section[] }
  const tabs = $derived.by((): Tab[] => {
    if (!frm) return [];
    const out: Tab[] = [{ label: __("Details"), sections: [] }];
    const newSection = (f?: Field): Section => ({ label: f?.label, collapsible: f?.collapsible, fields: [], dependsOn: f?.dependsOn });
    let tab = out[0];
    let sec: Section | null = null;
    for (const f of frm.meta.doctype.fields) {
      const fx = frm.field(f.fieldname || "") || f;
      if (f.fieldtype === "Tab Break") { tab = { label: f.label || "", sections: [] }; out.push(tab); sec = null; continue; }
      if (f.fieldtype === "Section Break") { sec = newSection(fx); tab.sections.push(sec); continue; }
      if (!sec) { sec = newSection(); tab.sections.push(sec); }
      sec.fields.push(fx);
    }
    return out;
  });

  $effect(() => {
    if (tabs.length <= 1) return;
    const nextTab = resolveActiveTab(tabs, page.url.searchParams.get("tab"));
    if (activeTab !== nextTab) activeTab = nextTab;
  });

  function switchTab(i: number) {
    activeTab = i;
    if (typeof window === "undefined") return;
    const params = tabToSearchParams(tabs, i, page.url.searchParams);
    const search = params.size ? `?${params}` : "";
    const next = `${page.url.pathname}${search}${page.url.hash}`;
    if (next !== `${page.url.pathname}${page.url.search}${page.url.hash}`) {
      goto(next, { replaceState: true, noScroll: true, keepFocus: true });
    }
  }

  /**
   * A tree's parent field offers groups only, never the document itself or one
   * of its descendants (DAT-07) — the three rules the server enforces on save,
   * applied here so the picker does not offer what the save would refuse. An
   * app's own setQuery for the field wins over it.
   */
  function parentQuery(f: Field) {
    if (!frm || !frm.meta.doctype.isTree || f.fieldname !== frm.meta.doctype.parentField) return undefined;
    return () => treeParentQuery(frm!.doc);
  }

  const statusField = $derived(frm?.meta.doctype.fields.find((f) => f.fieldname === "status"));
  /** The canonical status value — what the colour is keyed on. */
  const status = $derived.by(() => {
    if (!frm) return "";
    if (statusField && frm.doc[statusField.fieldname!]) return frm.doc[statusField.fieldname!];
    if (frm.isNew) return "New";
    if (frm.isSubmittable) return frm.docstatus === 2 ? "Cancelled" : frm.docstatus === 1 ? "Submitted" : "Draft";
    return "";
  });
  /** The same status as the reader sees it. */
  const statusLabel = $derived.by(() => {
    if (!status) return "";
    if (statusField && frm?.doc[statusField.fieldname!]) {
      const i = selectOptions(statusField).indexOf(status);
      if (i >= 0) return selectLabels(statusField)[i];
    }
    return __(status);
  });
  const title = $derived(
    frm
      ? frm.isSingle ? frm.meta.doctype.label : frm.isNew
        ? frm.doc.id?.trim() || __("New {0}", [frm.meta.doctype.label])
        : (frm.meta.doctype.titleField && frm.doc[frm.meta.doctype.titleField]) || (frm.meta.doctype.translateId ? __(frm.doc.id) : frm.doc.id)
      : ""
  );

  const canRename = $derived(!frm?.isNew && !!frm?.meta.doctype.allowRename && !!frm?.perm.write && frm?.docstatus === 0);
  const hasTitleField = $derived(!frm?.isNew && !!frm?.meta.doctype.titleField && !!frm?.perm.write && frm?.docstatus === 0);
  const canEditTitle = $derived(canRename || hasTitleField);
  const editTitleTooltip = $derived(
    hasTitleField
      ? __("Edit {0}", [frm?.field(frm?.meta.doctype.titleField!)?.label || __("title")])
      : __("Rename {0}", [frm?.meta.doctype.label || doctype])
  );

  async function onEditTitle() {
    if (!frm) return;
    if (hasTitleField && frm.meta.doctype.titleField) {
      const tf = frm.meta.doctype.titleField;
      const fDef = frm.field(tf);
      const label = fDef?.label || __("Title");
      const v = await prompt(__("Edit {0}", [label]), [
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
    if (!frm || !(await confirm(__("Delete {0}?", [escapeHtml(title)]), __("Delete"), { destructive: true }))) return;
    leaving = true; // the record is going away: unsaved edits go with it
    try {
      await frm.delete();
      gone = true;
      clearDraft(drafts, currentDraftKey());
    } catch (e) { leaving = false; showError(e); }
  }
  async function rename() {
    if (!frm) return;
    const label = frm.meta.doctype.label || frm.meta.doctype.name;
    const v = await prompt(__("Rename {0}", [label]), [
      { fieldname: "id", fieldtype: "Data", label: frm.meta.doctype.idLabel || __("New ID"), reqd: true, default: frm.doc.id }
    ]);
    if (!v || !v.id || v.id.trim() === frm.doc.id) return;
    try {
      const nn = await api.docMethod(doctype, frm.doc.id, "rename", { id: v.id.trim() });
      leaving = true; // a full page load, deliberate: the guard has nothing to ask
      location.href = `${basePath}/${encodeURIComponent(nn)}`;
    } catch (e) { showError(e); }
  }
  async function duplicate() {
    if (!frm) return;
    const copy = { ...frm.doc, id: undefined, __islocal: true, docstatus: 0, creation: undefined, modified: undefined, owner: undefined, amended_from: undefined };
    for (const f of frm.meta.doctype.fields) if (isTableType(f.fieldtype)) copy[f.fieldname!] = (copy[f.fieldname!] || []).map((r: any) => ({ ...r, id: undefined, parent: undefined }));
    frm.load(copy);
    duplicated = true; // the URL says /new now, and so must the draft
    history.replaceState(null, "", `${basePath}/new`);
    await frm.runRefresh();
    toast(__("Copy created — save it to keep it"), { indicator: "blue" });
  }
  function openPrint() {
    if (!frm || frm.isNew) return;
    goto(`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(id)}/print`);
  }
  // dropdowns close on any click outside them (mouseleave used to need two clicks)
  function onPointerDown(e: PointerEvent) {
    if (!(menuOpen || openGroup || workflowMenuOpen)) return;
    if ((e.target as HTMLElement | null)?.closest(".dropdown")) return;
    menuOpen = false;
    openGroup = "";
    workflowMenuOpen = false;
  }
  function onKey(e: KeyboardEvent) {
    if (e.key === "Escape") { menuOpen = false; openGroup = ""; workflowMenuOpen = false; }
    if ((e.ctrlKey || e.metaKey) && e.key === "s") {
      e.preventDefault();
      if (!frm || frm.readOnly) return;
      // the field being typed only commits when it loses focus, and the
      // shortcut does not move focus: flush it before reading the document
      commitFocusedEdit(document.activeElement);
      frm.save();
    }
  }

  async function handleWorkflowAction(action: string) {
    if (!frm) return;
    // the document's title, not its id: a hash id tells the reader nothing
    if (await confirm(__('{0} "{1}"?', [escapeHtml(__(action)), escapeHtml(title)]), __(action))) {
      await frm.applyWorkflowAction(action);
    }
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
        <div class="small muted"><a href={basePath}>{frm.meta.doctype.label}</a></div>
        <h1 style="display:flex;align-items:center;gap:8px">
          {#if canEditTitle}
            <button type="button" class="title-edit-btn" onclick={onEditTitle} title={editTitleTooltip}>
              <span>{title}</span>
              <span class="title-edit-icon" aria-hidden="true"><Icon name="edit-2" size={16} /></span>
            </button>
          {:else}
            <span>{title}</span>
          {/if}
          {#if frm.workflow?.state}
            <span class="indicator workflow-state {statusColor(frm.workflow.state, statusField)}">{__(frm.workflow.state)}</span>
          {:else if status}
            <span class="indicator {statusColor(status, statusField)}">{statusLabel}</span>
          {/if}
          {#if frm.isDirty && !frm.isNew}<span class="indicator orange">{__("Not saved")}</span>{/if}
        </h1>
      </div>
      <span class="spacer"></span>
      {#if stale}<button class="btn" onclick={() => { stale = false; frm?.reload(); }}><Icon name="refresh-cw" size={14} />{__("Changed by someone else — reload")}</button>{/if}
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
        <button class="btn icon" title={__("Print")} onclick={openPrint} aria-label={__("Print")}><Icon name="printer" /></button>
        <div class="dropdown">
          <button class="btn icon" onclick={() => (menuOpen = !menuOpen)} aria-label="Menu"><Icon name="more-horizontal" /></button>
          {#if menuOpen}
            <div class="menu" role="menu" tabindex="-1">
              <button onclick={() => { menuOpen = false; openPrint(); }}>{__("Print")}</button>
              <button onclick={() => { menuOpen = false; frm?.reload(); }}>{__("Reload")}</button>
              {#if !frm.isSingle && frm.perm.create}<button onclick={() => { menuOpen = false; duplicate(); }}>{__("Duplicate")}</button>{/if}
              {#if frm.meta.doctype.allowRename && frm.perm.write && frm.docstatus === 0}<button onclick={() => { menuOpen = false; rename(); }}>{__("Rename")}</button>{/if}
              {#if !frm.isSingle && frm.perm.delete && frm.docstatus !== 1}<button class="danger" style="color:var(--red)" onclick={() => { menuOpen = false; remove(); }}>{__("Delete")}</button>{/if}
              <button onclick={() => { menuOpen = false; openShortcutsHelp(); }} style="display:flex;align-items:center;justify-content:space-between">
                <span>{__("Keyboard shortcuts")}</span>
                <kbd class="kbd">?</kbd>
              </button>
            </div>
          {/if}
        </div>
      {/if}
      {#if frm.isDirty}<button class="btn" disabled={frm.saving} onclick={discard}>{__("Discard")}</button>{/if}
      {#if frm.workflow?.actions && frm.workflow.actions.length > 0 && !frm.isNew}
        {#if frm.workflow.actions.length > 2}
          <div class="dropdown">
            <button class="btn primary" disabled={frm.saving || frm.isDirty} onclick={() => (workflowMenuOpen = !workflowMenuOpen)}>{__("Actions")} <Icon name="chevron-down" size={14} /></button>
            {#if workflowMenuOpen}
              <div class="menu" role="menu" tabindex="-1">
                {#each frm.workflow.actions as act}
                  <button onclick={async () => {
                    workflowMenuOpen = false;
                    await handleWorkflowAction(act.action);
                  }}>{__(act.action)}</button>
                {/each}
              </div>
            {/if}
          </div>
        {:else}
          {#each frm.workflow.actions as act, i}
            <button class="btn" class:primary={i === 0} disabled={frm.saving || frm.isDirty} onclick={() => handleWorkflowAction(act.action)}>{__(act.action)}</button>
          {/each}
        {/if}
      {/if}
      {#if frm.primaryAction}
        <button class="btn primary" onclick={frm.primaryAction.action}>{frm.primaryAction.label}</button>
      {:else if frm.isSubmittable}
        {#if frm.docstatus === 0}
          {#if frm.isDirty || frm.isNew}
            {#if !frm.readOnly}
              <button class="btn primary" disabled={frm.saving} onclick={() => frm?.save()} title="{__('Save')} ({modKey}+S)">{__("Save")}<kbd class="btn-kbd">{modKey}S</kbd></button>
            {/if}
          {:else if !frm.workflow && frm.perm.submit}
            <button class="btn primary" disabled={frm.saving} onclick={async () => (await confirm(__("Submit {0} permanently?", [escapeHtml(title)]), __("Submit"))) && frm?.submit()}>{__("Submit")}</button>
          {/if}
        {:else if frm.docstatus === 1}
          {#if frm.isDirty}
            {#if !frm.readOnly}
              <button class="btn primary" disabled={frm.saving} onclick={() => frm?.save()} title="{__('Update')} ({modKey}+S)">{__("Update")}<kbd class="btn-kbd">{modKey}S</kbd></button>
            {/if}
          {:else if !frm.workflow && frm.perm.cancel}
            <button class="btn" disabled={frm.saving} onclick={async () => (await confirm(__("Cancel {0}?", [escapeHtml(title)]), __("Cancel"), { destructive: true })) && frm?.cancel()}>{__("Cancel")}</button>
          {/if}
        {:else if frm.perm.amend}
          <button class="btn primary" onclick={() => frm?.amend()}>{__("Amend")}</button>
        {/if}
      {:else if !frm.readOnly && (frm.perm.write || (!frm.isSingle && frm.isNew && frm.perm.create))}
        {#if !frm.workflow || frm.isDirty || frm.isNew}
          <button class="btn primary" disabled={frm.saving || (!frm.isDirty && !frm.isNew)} onclick={() => frm?.save()} title="{__('Save')} ({modKey}+S)">{__("Save")}<kbd class="btn-kbd">{modKey}S</kbd></button>
        {/if}
      {/if}
    </div>

    <div class="form-body">
      <div class="form-main">
        {#if tabs.length > 1}
          <div class="tabs">{#each tabs as t, i}<button class:active={activeTab === i} onclick={() => switchTab(i)}>{t.label}</button>{/each}</div>
        {/if}
        {#each tabs[activeTab]?.sections || [] as sec, si}
          {#if visibleSection(sec) && sec.fields.some((f) => frm!.isFieldVisible(f))}
            <div class="form-section">
              {#if sec.label}
                {#if sec.collapsible}
                  <h3><button type="button" class="section-toggle" aria-expanded={!isSectionCollapsed(collapsed, si)} onclick={() => toggleSection(collapsed, si)}>{sec.label} <Icon name={isSectionCollapsed(collapsed, si) ? "chevron-right" : "chevron-down"} size={12} /></button></h3>
                {:else}
                  <h3>{sec.label}</h3>
                {/if}
              {/if}
              {#if !(sec.collapsible && isSectionCollapsed(collapsed, si))}
                {#each packLines(sec.fields, (f) => frm!.isFieldVisible(f)) as line}
                  <div class="form-row">
                    {#each line as cell}
                      {@const f = cell.field}
                      <div class="form-cell {cellWidthClass(cell.slots, LINE_SLOTS)}">
                        {#if !f}
                          <!-- keeps the next control aligned to its half of the line -->
                        {:else if f.fieldtype === "Table"}
                          <Grid {frm} field={f} childMeta={frm.meta.children[f.options]} />
                        {:else if f.fieldtype === "HTML"}
                          <div class="field">{@html f.options || ""}</div>
                        {:else}
                          <Control field={f} value={frm.doc[f.fieldname!]} onchange={(v) => frm?.setValue(f.fieldname!, v)} doc={frm.doc}
                            readOnly={!frm.isFieldEditable(f)} mandatory={frm.isFieldMandatory(f)} error={frm.fieldErrors[f.fieldname!] || ""} query={frm.queries.get(f.fieldname!) || parentQuery(f)}
                            buttons={frm.fieldButtons[f.fieldname!] || []} />
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
  <div class="page"><Spinner /></div>
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
