<script lang="ts">
  // Right column of the form: metadata, assignments, shares, comments, versions.
  import type { FormController } from "$lib/form.svelte";
  import { api, type AssignArgs, type ShareArgs } from "$lib/api";
  import { __, boot } from "$lib/boot.svelte";
  import { formatDatetime, timeAgo } from "$lib/format";
  import { showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
  import Icon from "./Icon.svelte";
  import DocHistory from "./DocHistory.svelte";
  import AssignModal from "./AssignModal.svelte";
  import ShareModal from "./ShareModal.svelte";
  import { DocSharesState } from "$lib/shares.svelte";
  import { shareRightLabels, canRemoveShare } from "./doc-sidebar-share";
  import { fromPlainText, normalizeRichText, sanitizeHtml } from "$lib/richtext";
  import { getModifierKey } from "$lib/shortcuts.svelte";
  import { DocAssignments } from "$lib/assignments.svelte";
  import { isAssignmentOverdue, assignmentInitial, priorityBadgeClass } from "./doc-sidebar-assignment";

  let { frm }: { frm: FormController } = $props();
  let comments = $state<any[]>([]);
  let versions = $state<any[]>([]);
  let text = $state("");
  let showVersions = $state(false);
  let showModal = $state(false);
  let showAssignModal = $state(false);
  let showShareModal = $state(false);
  const modKey = $derived(getModifierKey());

  const assignState = new DocAssignments("", "");
  const shareState = new DocSharesState("", "");

  async function load() {
    try {
      comments = await api.comments(frm.doctype, frm.doc.id);
      if (frm.meta.doctype.trackChanges) versions = await api.versions(frm.doctype, frm.doc.id);
      if (!frm.isNew) {
        assignState.doctype = frm.doctype;
        assignState.id = frm.doc.id;
        await assignState.load();
        if (!frm.meta.doctype.isSingle) {
          shareState.doctype = frm.doctype;
          shareState.id = frm.doc.id;
          await shareState.load();
        }
      }
    } catch {}
  }
  onMount(load);
  $effect(() => { frm.doc.modified; frm.doc.id; load(); });

  async function addComment() {
    if (!text.trim()) return;
    try {
      // the box is plain text, and saying so is what keeps "a<b and b>c" from
      // being read as a tag by the server's markup detection
      await api.insert("Comment", { reference_doctype: frm.doctype, reference_id: frm.doc.id, content: fromPlainText(text), comment_type: "Comment" });
      text = "";
      await load();
    } catch (e) { showError(e); }
  }

  async function handleAssign(args: AssignArgs) {
    await assignState.assign(args);
    comments = await api.comments(frm.doctype, frm.doc.id);
  }

  async function handleShare(args: ShareArgs) {
    await shareState.add(args);
  }

  async function removeShare(user: string) {
    try {
      await shareState.remove(user);
    } catch (e) {
      showError(e);
    }
  }

  async function completeTask(id: string) {
    try {
      await assignState.complete(id);
      comments = await api.comments(frm.doctype, frm.doc.id);
    } catch (e) {
      showError(e);
    }
  }

  async function revokeTask(id: string) {
    try {
      await assignState.revoke(id);
      comments = await api.comments(frm.doctype, frm.doc.id);
    } catch (e) {
      showError(e);
    }
  }
</script>

<aside class="doc-sidebar">
  <!-- a Single never saved has nobody and no date to show -->
  {#if frm.doc.creation}
    <div class="small muted">
      <div>{__("Created by")} <b>{frm.doc.owner}</b> · {formatDatetime(frm.doc.creation)}</div>
      <div>{__("Modified by")} <b>{frm.doc.modified_by}</b> · {timeAgo(frm.doc.modified)}</div>
    </div>
  {/if}

  {#if !frm.isNew}
    <div class="block">
      <div class="assignments-head">
        <h4>{__("Assigned To")}</h4>
        <button
          type="button"
          class="btn sm btn-assign"
          onclick={() => (showAssignModal = true)}
          aria-label={__("Assign")}
        >
          <Icon name="plus" size={13} />
          <span>{__("Assign")}</span>
        </button>
      </div>

      {#if assignState.loading}
        <div class="small muted">{__("Loading...")}</div>
      {:else if assignState.rows.length === 0}
        <div class="small muted">{__("No active assignments")}</div>
      {:else}
        <div class="assignments-list">
          {#each assignState.rows as todo}
            <div class="assignment-item" class:closed={todo.status !== "Open"}>
              <div class="assignment-main">
                <div class="assignment-user">
                  <span class="avatar-sm">{assignmentInitial(todo.allocated_to)}</span>
                  <span class="small user-name" title={todo.allocated_to}>{todo.allocated_to}</span>
                </div>
                {#if todo.description}
                  <div class="small muted assignment-desc">{todo.description}</div>
                {/if}
                <div class="assignment-meta small muted">
                  {#if todo.date}
                    <span class="due-date" class:overdue={isAssignmentOverdue(todo.date, todo.status)}>
                      <Icon name="calendar" size={11} /> {todo.date}
                    </span>
                  {/if}
                  <span class="priority-badge {priorityBadgeClass(todo.priority)}">{__(todo.priority)}</span>
                </div>
              </div>
              {#if todo.status === "Open"}
                <div class="assignment-actions">
                  <button
                    type="button"
                    class="btn icon sm"
                    title={__("Complete")}
                    aria-label={__("Complete")}
                    disabled={assignState.pending === todo.id}
                    onclick={() => completeTask(todo.id)}
                  >
                    <Icon name="check" size={13} />
                  </button>
                  <button
                    type="button"
                    class="btn icon sm"
                    title={__("Revoke")}
                    aria-label={__("Revoke")}
                    disabled={assignState.pending === todo.id}
                    onclick={() => revokeTask(todo.id)}
                  >
                    <Icon name="x" size={13} />
                  </button>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  {#if !frm.isNew && !frm.meta.doctype.isSingle && (shareState.canShare || shareState.rows.length > 0)}
    <div class="block">
      <div class="assignments-head">
        <h4>{__("Shared With")}</h4>
        {#if shareState.canShare}
          <button
            type="button"
            class="btn sm btn-assign"
            onclick={() => (showShareModal = true)}
            aria-label={__("Share")}
          >
            <Icon name="share-2" size={13} />
            <span>{__("Share")}</span>
          </button>
        {/if}
      </div>

      {#if shareState.rows.length === 0}
        <div class="small muted">{__("Not shared with anyone")}</div>
      {:else}
        <div class="assignments-list">
          {#each shareState.rows as share (share.user)}
            <div class="assignment-item">
              <div class="assignment-main">
                <div class="assignment-user">
                  <span class="avatar-sm">{assignmentInitial(share.user)}</span>
                  <span class="small user-name" title={share.user}>{share.user}</span>
                </div>
                <div class="assignment-meta small muted">
                  {#each shareRightLabels(share) as right}
                    <span class="priority-badge priority-low">{__(right)}</span>
                  {/each}
                  {#if share.override_scope}
                    <span class="priority-badge priority-high" title={__("Override Security Scope")}>
                      <Icon name="shield" size={10} />
                    </span>
                  {/if}
                </div>
              </div>
              {#if canRemoveShare(share, shareState.canShare, boot.data?.user ?? "")}
                <div class="assignment-actions">
                  <button
                    type="button"
                    class="btn icon sm"
                    title={__("Remove share")}
                    aria-label={__("Remove share")}
                    disabled={shareState.pending === share.user}
                    onclick={() => removeShare(share.user)}
                  >
                    <Icon name="x" size={13} />
                  </button>
                </div>
              {/if}
            </div>
          {/each}
        </div>
      {/if}
    </div>
  {/if}

  <div class="block">
    <h4>{__("Comments")}</h4>
    {#each comments as c}
      <!-- a comment is rich text (DAT-08): the server cleans what it stores,
           and this cleans again, because a row may predate that or come from
           a direct SQL write -->
      <div class="comment"><div class="small muted"><b>{c.owner}</b> · {timeAgo(c.creation)}</div><div class="comment-body">{@html sanitizeHtml(normalizeRichText(String(c.content ?? "")))}</div></div>
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
          <h3>{__("Change history")} · {frm.doc.id}</h3>
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

{#if showShareModal}
  <ShareModal
    open={showShareModal}
    canOverrideScope={shareState.canOverrideScope}
    onshare={handleShare}
    onclose={() => (showShareModal = false)}
  />
{/if}

{#if showAssignModal}
  <AssignModal
    open={showAssignModal}
    doctype={frm.doctype}
    docId={frm.doc.id}
    onassign={handleAssign}
    onclose={() => (showAssignModal = false)}
  />
{/if}

<style>
  .comment-body { font-size: 13px; line-height: 1.5; }
  .comment-body :global(p) { margin: 0 0 6px; }
  .comment-body :global(:last-child) { margin-bottom: 0; }
  .comment-body :global(ul), .comment-body :global(ol) { margin: 0 0 6px; padding-left: 18px; }
  .comment-body :global(img) { max-width: 100%; border-radius: 4px; }
  .doc-sidebar { width: 260px; flex-shrink: 0; }
  .block { margin-top: 18px; }
  h4 { font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); margin: 0 0 8px; }
  .comment { padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }

  /* Assignments */
  .assignments-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
  }
  .assignments-head h4 {
    margin: 0;
  }
  .btn-assign {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    font-size: 11px;
    cursor: pointer;
  }
  .assignments-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .assignment-item {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 8px;
    padding: 8px;
    background: var(--surface, #fff);
    border: 1px solid var(--border);
    border-radius: 6px;
    font-size: 12px;
  }
  .assignment-item.closed {
    opacity: 0.6;
  }
  .assignment-main {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
    flex: 1;
  }
  .assignment-user {
    display: flex;
    align-items: center;
    gap: 6px;
  }
  .avatar-sm {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 20px;
    height: 20px;
    border-radius: 50%;
    background: var(--primary, #2563eb);
    color: #fff;
    font-size: 10px;
    font-weight: 600;
    flex-shrink: 0;
  }
  .user-name {
    font-weight: 500;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .assignment-desc {
    word-break: break-word;
    font-size: 11px;
    color: var(--muted);
  }
  .assignment-meta {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-top: 2px;
  }
  .due-date {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
  }
  .due-date.overdue {
    color: var(--danger, #dc2626);
    font-weight: 600;
  }
  .priority-badge {
    display: inline-block;
    padding: 1px 6px;
    border-radius: 999px;
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
  }
  .priority-urgent {
    background: #fee2e2;
    color: #b91c1c;
  }
  .priority-high {
    background: #ffedd5;
    color: #c2410c;
  }
  .priority-medium {
    background: #fef3c7;
    color: #b45309;
  }
  .priority-low {
    background: #f1f5f9;
    color: #475569;
  }
  .assignment-actions {
    display: flex;
    gap: 2px;
    flex-shrink: 0;
  }

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
