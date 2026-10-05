<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  // Right column of the form: metadata, assignments, shares, comments, versions.
  import type { FormController } from "$lib/form.svelte";
  import { api, type AssignArgs, type ShareArgs } from "$lib/api";
  import { __, boot, hasRole } from "$lib/boot.svelte";
  import { formatDatetime, timeAgo } from "$lib/format";
  import { confirm, showError } from "$lib/ui.svelte";
  import { onMount } from "svelte";
  import Icon from "./Icon.svelte";
  import DocHistory from "./DocHistory.svelte";
  import AssignModal from "./AssignModal.svelte";
  import ShareModal from "./ShareModal.svelte";
  import { DocSharesState } from "$lib/shares.svelte";
  import { shareRightLabels, canRemoveShare } from "./doc-sidebar-share";
  import { fromPlainText, htmlToText, normalizeRichText, sanitizeHtml } from "$lib/richtext";
  import { canDeleteComment, canEditComment, isCommentEdited } from "./doc-sidebar-comment";
  import { getModifierKey } from "$lib/shortcuts.svelte";
  import { DocAssignments } from "$lib/assignments.svelte";
  import { isAssignmentOverdue, assignmentInitial, priorityBadgeClass } from "./doc-sidebar-assignment";
  import { panels, setDocSidebarCollapsed } from "$lib/panels.svelte";
  import { attachmentsQuery, hasLooseAttachments, isAudioFile, looseAttachments, type AttachedFile } from "./doc-sidebar-attachments";
  import { formatFileSize } from "$lib/controls/attach-state";

  let { frm }: { frm: FormController } = $props();
  let comments = $state<any[]>([]);
  let versions = $state<any[]>([]);
  let attachments = $state<AttachedFile[]>([]);
  let text = $state("");
  let editingId = $state("");
  let editText = $state("");
  let commentPending = $state("");
  let showVersions = $state(false);
  let showModal = $state(false);
  let showAssignModal = $state(false);
  let showShareModal = $state(false);
  const modKey = $derived(getModifierKey());

  const assignState = new DocAssignments("", "");
  const shareState = new DocSharesState("", "");

  const collapsed = $derived(panels.docSidebarCollapsed);
  const showShares = $derived(!frm.isNew && !frm.meta.doctype.isSingle && (shareState.canShare || shareState.rows.length > 0));

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
  // apart from load(): a user who may not read File still sees the rest.
  // Only a Feedback lists its files: they reach it with no field to show
  // them, and every other form is spared the request.
  async function loadAttachments() {
    if (!hasLooseAttachments(frm.doctype) || frm.isNew || frm.meta.doctype.isSingle || !frm.doc.id) { attachments = []; return; }
    try {
      attachments = looseAttachments(await api.list("File", attachmentsQuery(frm.doctype, frm.doc.id)));
    } catch { attachments = []; }
  }
  onMount(load);
  $effect(() => { frm.doc.id; frm.isNew; loadAttachments(); });
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

  function startEdit(c: any) {
    editingId = c.id;
    // the box is plain text: a comment written with formatting loses it here
    editText = htmlToText(String(c.content ?? ""));
  }

  async function saveEdit() {
    if (!editText.trim() || commentPending) return;
    commentPending = editingId;
    try {
      await api.update("Comment", editingId, { content: fromPlainText(editText) });
      editingId = "";
      await load();
    } catch (e) { showError(e); }
    commentPending = "";
  }

  function editKeydown(e: KeyboardEvent) {
    if ((e.ctrlKey || e.metaKey) && e.key === "Enter") saveEdit();
    else if (e.key === "Escape") { e.stopPropagation(); editingId = ""; }
  }

  async function deleteComment(id: string) {
    if (!(await confirm(__("Delete this comment?"), __("Delete"), { destructive: true }))) return;
    commentPending = id;
    try {
      await api.remove("Comment", id);
      if (editingId === id) editingId = "";
      await load();
    } catch (e) { showError(e); }
    commentPending = "";
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

{#snippet railButton(icon: string, label: string, count: number)}
  <button type="button" class="rail-btn" title={label} aria-label={label} onclick={() => setDocSidebarCollapsed(false)}>
    <Icon name={icon} size={16} />
    {#if count > 0}<span class="count-badge">{count}</span>{/if}
  </button>
{/snippet}

<aside class="doc-sidebar" class:collapsed>
  <div class="panel-head">
    <button
      type="button"
      class="btn icon sm panel-toggle"
      onclick={() => setDocSidebarCollapsed(!collapsed)}
      title={collapsed ? __("Expand panel") : __("Collapse panel")}
      aria-label={collapsed ? __("Expand panel") : __("Collapse panel")}
      aria-expanded={!collapsed}
    >
      <Icon name="panel-right" size={14} />
    </button>
  </div>

  {#if collapsed}
    <div class="rail">
      {#if !frm.isNew}{@render railButton("users", __("Assigned To"), assignState.rows.length)}{/if}
      {#if showShares}{@render railButton("share-2", __("Shared With"), shareState.rows.length)}{/if}
      {@render railButton("message-square", __("Comments"), comments.length)}
      {#if frm.meta.doctype.trackChanges}{@render railButton("history", __("History"), versions.length)}{/if}
    </div>
  {/if}

  <!-- hidden rather than unmounted when collapsed: a half-written comment stays -->
  <div class="doc-sidebar-content">
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

  {#if showShares}
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

  {#if attachments.length}
    <div class="block">
      <h4>{__("Attachments")}</h4>
      <ul class="attachments">
        {#each attachments as a (a.id)}
          <li>
            <a href={a.file_url} target="_blank" rel="noopener" class="attachment-link" title={a.file_name || a.file_url}>
              <Icon name="paperclip" size={12} />
              <span class="attachment-name">{a.file_name || a.file_url.split("/").pop()}</span>
              {#if formatFileSize(a.file_size)}<span class="muted">{formatFileSize(a.file_size)}</span>{/if}
            </a>
            {#if isAudioFile(a.file_name) || isAudioFile(a.file_url)}
              <audio controls preload="none" src={a.file_url}></audio>
            {/if}
          </li>
        {/each}
      </ul>
    </div>
  {/if}

  <div class="block">
    <h4>{__("Comments")}</h4>
    {#each comments as c (c.id)}
      {@const me = boot.data?.user ?? ""}
      <div class="comment">
        <div class="comment-head">
          <div class="small muted comment-meta">
            <b>{c.owner}</b> · {timeAgo(c.creation)}{#if isCommentEdited(c)}
              · <span title={formatDatetime(c.modified)}>{__("edited")}</span>{/if}
          </div>
          {#if editingId !== c.id}
            <div class="assignment-actions">
              {#if canEditComment(c, me)}
                <button type="button" class="btn icon sm" title={__("Edit")} aria-label={__("Edit")} disabled={commentPending === c.id} onclick={() => startEdit(c)}>
                  <Icon name="pencil" size={13} />
                </button>
              {/if}
              {#if canDeleteComment(c, me, hasRole("System Manager"))}
                <button type="button" class="btn icon sm" title={__("Delete")} aria-label={__("Delete")} disabled={commentPending === c.id} onclick={() => deleteComment(c.id)}>
                  <Icon name="trash" size={13} />
                </button>
              {/if}
            </div>
          {/if}
        </div>
        {#if editingId === c.id}
          <!-- svelte-ignore a11y_autofocus -->
          <textarea class="input" rows="2" bind:value={editText} onkeydown={editKeydown} autofocus></textarea>
          <div class="comment-edit-actions">
            <button type="button" class="btn sm primary" disabled={!editText.trim() || commentPending === c.id} onclick={saveEdit} title="{__('Save')} ({modKey}+Enter)">{__("Save")}</button>
            <button type="button" class="btn sm" onclick={() => (editingId = "")}>{__("Cancel")}</button>
          </div>
        {:else}
          <!-- a comment is rich text (DAT-08): the server cleans what it stores,
               and this cleans again, because a row may predate that or come from
               a direct SQL write -->
          <div class="comment-body">{@html sanitizeHtml(normalizeRichText(String(c.content ?? "")))}</div>
        {/if}
      </div>
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
  </div>
</aside>

{#if showModal}
  <div
    class="modal-bg"
    use:focusTrap
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
  .panel-head { display: flex; justify-content: flex-end; margin-bottom: 4px; }
  .panel-toggle { color: var(--muted); }
  /* Collapsed: a rail of icons with each section's count. */
  .doc-sidebar.collapsed { width: 36px; }
  .doc-sidebar.collapsed .panel-head { justify-content: center; }
  .doc-sidebar.collapsed .doc-sidebar-content { display: none; }
  .rail { display: flex; flex-direction: column; gap: 4px; margin-top: 6px; }
  .rail-btn { display: flex; flex-direction: column; align-items: center; gap: 3px; width: 100%; padding: 7px 0; border: 0; background: none; border-radius: 6px; color: var(--muted); cursor: pointer; }
  .rail-btn:hover { background: #f3f4f6; color: var(--text); }
  .block { margin-top: 18px; }
  .attachments { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 6px; font-size: 12px; }
  .attachment-link { display: flex; align-items: center; gap: 6px; min-width: 0; color: inherit; }
  .attachment-name { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .attachments audio { display: block; width: 100%; height: 32px; margin-top: 4px; }
  h4 { font-size: 12px; text-transform: uppercase; letter-spacing: .04em; color: var(--muted); margin: 0 0 8px; }
  .comment { padding: 8px 0; border-bottom: 1px solid var(--border); font-size: 13px; }
  .comment-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 6px; }
  .comment-meta { min-width: 0; overflow-wrap: anywhere; }
  .comment-edit-actions { display: flex; gap: 6px; margin: 6px 0 2px; }

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
