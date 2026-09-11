<script lang="ts">
  // Right column of the form: metadata, comments, versions.
  import type { FormController } from "$lib/form.svelte";
  import { api } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { formatDatetime, timeAgo } from "$lib/format";
  import { showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
  import Icon from "./Icon.svelte";
  import DocHistory from "./DocHistory.svelte";
  import { getModifierKey } from "$lib/shortcuts.svelte";

  let { frm }: { frm: FormController } = $props();
  let comments = $state<any[]>([]);
  let versions = $state<any[]>([]);
  let text = $state("");
  let showVersions = $state(false);
  let showModal = $state(false);
  const modKey = $derived(getModifierKey());

  async function load() {
    try {
      comments = await api.comments(frm.doctype, frm.doc.name);
      if (frm.meta.doctype.trackChanges) versions = await api.versions(frm.doctype, frm.doc.name);
    } catch {}
  }
  onMount(load);
  $effect(() => { frm.doc.modified; load(); });

  async function addComment() {
    if (!text.trim()) return;
    try {
      await api.insert("Comment", { reference_doctype: frm.doctype, reference_name: frm.doc.name, content: text, comment_type: "Comment" });
      text = "";
      await load();
    } catch (e) { showError(e); }
  }
</script>

<aside class="doc-sidebar">
  <div class="small muted">
    <div>{__("Created by")} <b>{frm.doc.owner}</b> · {formatDatetime(frm.doc.creation)}</div>
    <div>{__("Modified by")} <b>{frm.doc.modified_by}</b> · {timeAgo(frm.doc.modified)}</div>
  </div>

  <div class="block">
    <h4>{__("Comments")}</h4>
    {#each comments as c}
      <div class="comment"><div class="small muted"><b>{c.owner}</b> · {timeAgo(c.creation)}</div><div>{c.content}</div></div>
    {/each}
    <textarea class="input" rows="2" placeholder={__("Write a comment")} bind:value={text} onkeydown={(e) => (e.ctrlKey || e.metaKey) && e.key === "Enter" && addComment()}></textarea>
    <div style="display:flex;align-items:center;justify-content:space-between;margin-top:6px">
      <button class="btn sm" onclick={addComment} title="{__('Comment')} ({modKey}+Enter)">{__("Comment")}</button>
      <span class="small muted"><kbd class="kbd">{modKey}+Enter</kbd> {__("to submit")}</span>
    </div>
  </div>

  {#if frm.meta.doctype.trackChanges}
    <div class="block">
      <div class="history-head">
        <button
          type="button"
          class="section-toggle-btn"
          onclick={() => (showVersions = !showVersions)}
          aria-expanded={showVersions}
        >
          <Icon name={showVersions ? "chevron-down" : "chevron-right"} size={13} />
          <Icon name="history" size={14} />
          <span>{__("History")}</span>
          <span class="count-badge">{versions.length}</span>
        </button>

        {#if versions.length > 0}
          <button
            type="button"
            class="btn icon sm expand-head-btn"
            title={__("Open the full history")}
            onclick={() => (showModal = true)}
            aria-label={__("Open the full history")}
          >
            <Icon name="maximize-2" size={13} />
          </button>
        {/if}
      </div>

      {#if showVersions}
        <div class="history-body">
          <DocHistory {frm} {versions} mode="compact" onExpand={() => (showModal = true)} />
        </div>
      {/if}
    </div>
  {/if}
</aside>

{#if showModal}
  <div
    class="modal-bg"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    onkeydown={(e) => e.key === "Escape" && (showModal = false)}
  >
    <div class="modal lg">
      <div class="head">
        <div class="modal-title-wrap">
          <Icon name="history" size={18} />
          <h3>{__("Change history")} · {frm.doc.name}</h3>
        </div>
        <button class="btn icon" onclick={() => (showModal = false)} aria-label="Fechar">
          <Icon name="x" size={16} />
        </button>
      </div>
      <div class="body modal-scroll-body">
        <DocHistory {frm} {versions} mode="full" />
      </div>
      <div class="foot">
        <button class="btn" onclick={() => (showModal = false)}>{__("Close")}</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .doc-sidebar { width: 260px; flex-shrink: 0; }
  .block { margin-top: 18px; }
  h4 { font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); margin: 0 0 8px; }
  .comment { padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }

  /* History Header */
  .history-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 6px;
    margin-bottom: 8px;
  }
  .section-toggle-btn {
    display: flex;
    align-items: center;
    gap: 6px;
    background: none;
    border: none;
    padding: 2px 0;
    cursor: pointer;
    font-size: 12px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: .04em;
    color: var(--muted);
    transition: color 0.15s ease;
  }
  .section-toggle-btn:hover {
    color: var(--text);
  }
  .count-badge {
    background: #eef0f3;
    color: var(--text);
    padding: 1px 6px;
    border-radius: 999px;
    font-size: 11px;
    font-weight: 500;
  }
  .expand-head-btn {
    color: var(--muted);
    padding: 3px 5px;
  }
  .expand-head-btn:hover {
    color: var(--primary);
  }
  .history-body {
    margin-top: 6px;
  }

  /* Modal Title */
  .modal-title-wrap {
    display: flex;
    align-items: center;
    gap: 8px;
    flex: 1;
  }
  .modal-title-wrap h3 {
    font-size: 15px;
    margin: 0;
  }
  .modal-scroll-body {
    max-height: 72vh;
    overflow-y: auto;
    padding: 20px;
  }

  @media (max-width: 1000px) { .doc-sidebar { display: none; } }
</style>
