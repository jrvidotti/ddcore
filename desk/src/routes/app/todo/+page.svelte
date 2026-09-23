<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  import { onDestroy, onMount } from "svelte";
  import { PendingWork, pendingTasks, refreshPendingCount } from "$lib/assignments.svelte";
  import { api, type ToDoDoc } from "$lib/api";
  import { __, boot, doctypeLabel, siteName } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import LinkControl from "$lib/controls/LinkControl.svelte";
  import { isAssignmentOverdue, priorityBadgeClass } from "$lib/components/doc-sidebar-assignment";

  const pw = new PendingWork();
  let showNewModal = $state(false);

  // New ToDo form fields
  let newDescription = $state("");
  let newDate = $state("");
  let newPriority = $state<"Low" | "Medium" | "High" | "Urgent">("Medium");
  let newAllocatedTo = $state("");
  let savingNew = $state(false);

  const userField = {
    fieldname: "allocated_to",
    fieldtype: "Link",
    options: "User",
    label: "User",
  };

  onMount(() => {
    newAllocatedTo = boot.data?.user || "";
    void pw.load(0);
  });

  $effect(() => {
    pendingTasks.revision;
  });

  onDestroy(() => pw.destroy());

  function setScope(scope: "assigned_to_me" | "assigned_by_me") {
    pw.scope = scope;
    pw.offset = 0;
    void pw.load(0, pw.status, scope);
  }

  function setStatus(status: string) {
    pw.status = status;
    pw.offset = 0;
    void pw.load(0, status, pw.scope);
  }

  async function toggleComplete(row: ToDoDoc) {
    if (row.status === "Open") {
      await pw.complete(row.id);
    }
  }

  async function revokeTask(row: ToDoDoc) {
    await pw.revoke(row.id);
  }

  async function createStandaloneTask() {
    if (!newDescription.trim()) return;
    savingNew = true;
    try {
      await api.insert("ToDo", {
        description: newDescription.trim(),
        date: newDate || undefined,
        priority: newPriority,
        allocated_to: newAllocatedTo || boot.data?.user,
        status: "Open",
      });
      showNewModal = false;
      newDescription = "";
      newDate = "";
      newPriority = "Medium";
      await pw.load(pw.offset);
      await refreshPendingCount();
    } catch (e) {
      showError(e);
    } finally {
      savingNew = false;
    }
  }
</script>

<svelte:head><title>{__("To-Do")} · {siteName()}</title></svelte:head>

<div class="page todo-page">
  <div class="page-head">
    <h1>{__("To-Do")}</h1>
    <div class="head-actions">
      <button class="btn" onclick={() => pw.load(pw.offset)} disabled={pw.loading}>
        <Icon name="refresh-cw" />{__("Refresh")}
      </button>
      <button class="btn primary" onclick={() => (showNewModal = true)}>
        <Icon name="plus" />{__("New Task")}
      </button>
    </div>
  </div>

  <div class="toolbar">
    <div class="scope-tabs" role="tablist">
      <button
        role="tab"
        class="tab-btn"
        class:active={pw.scope === "assigned_to_me"}
        aria-selected={pw.scope === "assigned_to_me"}
        onclick={() => setScope("assigned_to_me")}
      >
        {__("Assigned to me")}
      </button>
      <button
        role="tab"
        class="tab-btn"
        class:active={pw.scope === "assigned_by_me"}
        aria-selected={pw.scope === "assigned_by_me"}
        onclick={() => setScope("assigned_by_me")}
      >
        {__("Assigned by me")}
      </button>
    </div>

    <div class="status-filter">
      <label for="status-select">{__("Status")}</label>
      <select
        id="status-select"
        class="input"
        bind:value={pw.status}
        onchange={(e) => setStatus((e.target as HTMLSelectElement).value)}
      >
        <option value="Open">{__("Open")}</option>
        <option value="Closed">{__("Closed")}</option>
        <option value="all">{__("All")}</option>
      </select>
    </div>
  </div>

  <section class="task-list-section" aria-label={__("To-Do List")} aria-busy={pw.loading}>
    {#if pw.error}
      <div class="state" role="alert">
        <p>{pw.error}</p>
        <button class="btn" onclick={() => pw.load(pw.offset)}>{__("Retry")}</button>
      </div>
    {:else if pw.loading}
      <p class="state muted" role="status">{__("Loading tasks…")}</p>
    {:else if pw.rows.length === 0}
      <div class="state muted">
        <Icon name="check-square" size={32} />
        <p>{__("No tasks found")}</p>
      </div>
    {:else}
      <ul class="task-list">
        {#each pw.rows as row (row.id)}
          <li class="task-row" class:completed={row.status === "Closed"}>
            <div class="task-check">
              <input
                type="checkbox"
                checked={row.status === "Closed"}
                disabled={row.status !== "Open" || pw.pending === row.id}
                onchange={() => toggleComplete(row)}
                aria-label={__("Mark as completed")}
              />
            </div>

            <div class="task-content">
              <div class="task-header">
                <span class="task-desc" class:done-text={row.status === "Closed"}>
                  {row.description || row.id}
                </span>
                <span class="priority-badge {priorityBadgeClass(row.priority)}">
                  {__(row.priority)}
                </span>
              </div>

              <div class="task-meta">
                {#if row.reference_type && row.reference_id}
                  <a
                    class="ref-doc"
                    href={`/app/${encodeURIComponent(row.reference_type)}/${encodeURIComponent(row.reference_id)}`}
                  >
                    <Icon name="file-text" size={12} />
                    {doctypeLabel(row.reference_type)} · {row.reference_id}
                  </a>
                {/if}

                {#if row.date}
                  <span class="due-pill" class:overdue={isAssignmentOverdue(row.date, row.status)}>
                    <Icon name="calendar" size={12} />
                    {row.date}
                  </span>
                {/if}

                <span class="assignee-text muted">
                  {#if pw.scope === "assigned_to_me"}
                    {__("Assigned by {0}", [row.assigned_by || "System"])}
                  {:else}
                    {__("Assigned to {0}", [row.allocated_to])}
                  {/if}
                </span>
              </div>
            </div>

            {#if row.status === "Open"}
              <div class="task-actions">
                <button
                  type="button"
                  class="btn icon sm"
                  title={__("Complete")}
                  aria-label={__("Complete")}
                  disabled={pw.pending === row.id}
                  onclick={() => toggleComplete(row)}
                >
                  <Icon name="check" size={14} />
                </button>
                <button
                  type="button"
                  class="btn icon sm"
                  title={__("Revoke")}
                  aria-label={__("Revoke")}
                  disabled={pw.pending === row.id}
                  onclick={() => revokeTask(row)}
                >
                  <Icon name="x" size={14} />
                </button>
              </div>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  </section>

  <div class="pagination">
    <span class="muted" aria-live="polite">{__("{0} tasks", [pw.total])}</span>
    <button
      class="btn"
      disabled={pw.loading || pw.offset === 0}
      onclick={() => { pw.offset = Math.max(0, pw.offset - pw.limit); pw.load(pw.offset); }}
    >
      {__("Previous")}
    </button>
    <button
      class="btn"
      disabled={pw.loading || pw.offset + pw.limit >= pw.total}
      onclick={() => { pw.offset += pw.limit; pw.load(pw.offset); }}
    >
      {__("Next")}
    </button>
  </div>
</div>

{#if showNewModal}
  <div
    class="modal-bg"
    use:focusTrap
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && (showNewModal = false)}
    onkeydown={(e) => e.key === "Escape" && (showNewModal = false)}
  >
    <div class="modal sm">
      <div class="head">
        <h3 style="display:flex;align-items:center;gap:8px">
          <Icon name="check-square" size={18} />
          {__("New Task")}
        </h3>
        <button class="btn icon" onclick={() => (showNewModal = false)} aria-label={__("Close")}>
          <Icon name="x" size={16} />
        </button>
      </div>
      <div class="body">
        <div class="form-group">
          <label for="new-task-desc" class="label reqd">{__("Description")}</label>
          <textarea
            id="new-task-desc"
            class="input"
            rows="3"
            placeholder={__("What needs to be done?")}
            bind:value={newDescription}
          ></textarea>
        </div>

        <div class="form-row" style="display:flex;gap:12px;margin-top:12px">
          <div class="form-group" style="flex:1">
            <label for="new-task-date" class="label">{__("Due Date")}</label>
            <input id="new-task-date" type="date" class="input" bind:value={newDate} />
          </div>
          <div class="form-group" style="flex:1">
            <label for="new-task-priority" class="label">{__("Priority")}</label>
            <select id="new-task-priority" class="input" bind:value={newPriority}>
              <option value="Low">{__("Low")}</option>
              <option value="Medium">{__("Medium")}</option>
              <option value="High">{__("High")}</option>
              <option value="Urgent">{__("Urgent")}</option>
            </select>
          </div>
        </div>

        <div class="form-group" style="margin-top:12px">
          <label for="new-task-assignee" class="label">{__("Assign to")}</label>
          <LinkControl
            id="new-task-assignee"
            field={userField}
            value={newAllocatedTo}
            onchange={(v) => (newAllocatedTo = v || "")}
          />
        </div>
      </div>
      <div class="foot">
        <button class="btn" onclick={() => (showNewModal = false)}>{__("Cancel")}</button>
        <button
          class="btn primary"
          disabled={!newDescription.trim() || savingNew}
          onclick={createStandaloneTask}
        >
          {savingNew ? __("Saving...") : __("Save")}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .todo-page {
    max-width: 1100px;
  }
  .page-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 24px;
  }
  .page-head h1 {
    margin: 0;
  }
  .head-actions {
    display: flex;
    gap: 8px;
  }
  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    flex-wrap: wrap;
    gap: 12px;
    margin-bottom: 16px;
  }
  .scope-tabs {
    display: flex;
    background: var(--border, #e2e8f0);
    padding: 2px;
    border-radius: 6px;
  }
  .tab-btn {
    border: none;
    background: transparent;
    padding: 6px 14px;
    border-radius: 4px;
    font-size: 13px;
    font-weight: 500;
    color: var(--muted, #64748b);
    cursor: pointer;
    transition: background 0.15s, color 0.15s;
  }
  .tab-btn.active {
    background: var(--surface, #fff);
    color: var(--text, #1e293b);
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.05);
  }
  .status-filter {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 13px;
  }
  .status-filter select {
    width: auto;
    min-width: 110px;
  }

  .task-list-section {
    border: 1px solid var(--border);
    border-radius: 8px;
    background: white;
    overflow: hidden;
  }
  .task-list {
    list-style: none;
    margin: 0;
    padding: 0;
  }
  .task-row {
    display: flex;
    align-items: flex-start;
    gap: 14px;
    padding: 16px 20px;
    border-bottom: 1px solid var(--border);
    transition: background 0.1s;
  }
  .task-row:last-child {
    border-bottom: 0;
  }
  .task-row:hover {
    background: #fcfdfe;
  }
  .task-row.completed {
    background: #fafafa;
    opacity: 0.7;
  }
  .task-check {
    margin-top: 2px;
  }
  .task-check input[type="checkbox"] {
    width: 16px;
    height: 16px;
    cursor: pointer;
  }
  .task-content {
    flex: 1;
    min-width: 0;
  }
  .task-header {
    display: flex;
    align-items: baseline;
    gap: 10px;
    margin-bottom: 6px;
  }
  .task-desc {
    font-size: 14px;
    font-weight: 500;
    word-break: break-word;
  }
  .task-desc.done-text {
    text-decoration: line-through;
    color: var(--muted);
  }
  .task-meta {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: 12px;
    font-size: 12px;
  }
  .ref-doc {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--primary, #2563eb);
    text-decoration: none;
    font-weight: 500;
  }
  .ref-doc:hover {
    text-decoration: underline;
  }
  .due-pill {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    color: var(--muted);
  }
  .due-pill.overdue {
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
  .task-actions {
    display: flex;
    gap: 4px;
    flex-shrink: 0;
  }
  .state {
    padding: 44px 20px;
    text-align: center;
  }
  .pagination {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: 12px;
    margin-top: 16px;
  }
  .pagination span {
    margin-right: auto;
  }
  .label {
    display: block;
    font-size: 12px;
    font-weight: 500;
    margin-bottom: 4px;
    color: var(--muted);
  }
  .label.reqd::after {
    content: " *";
    color: var(--danger, #e53e3e);
  }
  .form-group {
    display: flex;
    flex-direction: column;
  }
</style>
