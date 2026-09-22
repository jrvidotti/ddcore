<script lang="ts">
  import type { AssignArgs } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";
  import LinkControl from "$lib/controls/LinkControl.svelte";

  let {
    open = false,
    doctype,
    docId,
    onassign,
    onclose,
  }: {
    open: boolean;
    doctype: string;
    docId: string;
    onassign: (args: AssignArgs) => Promise<void>;
    onclose: () => void;
  } = $props();

  let allocatedTo = $state("");
  let date = $state("");
  let priority = $state<"Low" | "Medium" | "High" | "Urgent">("Medium");
  let description = $state("");
  let saving = $state(false);

  const userField = {
    fieldname: "allocated_to",
    fieldtype: "Link",
    options: "User",
    label: "User",
  };

  async function submit() {
    if (!allocatedTo) return;
    saving = true;
    try {
      await onassign({
        allocated_to: allocatedTo,
        date: date || undefined,
        priority,
        description: description.trim() || undefined,
      });
      onclose();
      allocatedTo = "";
      date = "";
      priority = "Medium";
      description = "";
    } catch (e) {
      showError(e);
    } finally {
      saving = false;
    }
  }
</script>

{#if open}
  <div
    class="modal-bg"
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && onclose()}
    onkeydown={(e) => e.key === "Escape" && onclose()}
  >
    <div class="modal sm">
      <div class="head">
        <h3 style="display:flex;align-items:center;gap:8px">
          <Icon name="user-plus" size={18} />
          {__("Assign document")}
        </h3>
        <button class="btn icon" onclick={onclose} aria-label={__("Close")}>
          <Icon name="x" size={16} />
        </button>
      </div>
      <div class="body">
        <div class="form-group">
          <label for="assignee-input" class="label reqd">{__("Assign to")}</label>
          <LinkControl
            id="assignee-input"
            field={userField}
            value={allocatedTo}
            onchange={(v) => (allocatedTo = v || "")}
          />
        </div>

        <div class="form-row" style="display:flex;gap:12px;margin-top:12px">
          <div class="form-group" style="flex:1">
            <label for="assign-due-date" class="label">{__("Due Date")}</label>
            <input id="assign-due-date" type="date" class="input" bind:value={date} />
          </div>
          <div class="form-group" style="flex:1">
            <label for="assign-priority" class="label">{__("Priority")}</label>
            <select id="assign-priority" class="input" bind:value={priority}>
              <option value="Low">{__("Low")}</option>
              <option value="Medium">{__("Medium")}</option>
              <option value="High">{__("High")}</option>
              <option value="Urgent">{__("Urgent")}</option>
            </select>
          </div>
        </div>

        <div class="form-group" style="margin-top:12px">
          <label for="assign-description" class="label">{__("Description")}</label>
          <textarea
            id="assign-description"
            class="input"
            rows="3"
            placeholder={__("Task instructions / notes")}
            bind:value={description}
          ></textarea>
        </div>
      </div>
      <div class="foot">
        <button class="btn" onclick={onclose}>{__("Cancel")}</button>
        <button class="btn primary" disabled={!allocatedTo || saving} onclick={submit}>
          {saving ? __("Assigning...") : __("Assign")}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
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
