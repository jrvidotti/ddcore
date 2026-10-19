<script lang="ts">
  // The assignment form: assign a document to a user (the assignee is required) or
  // create a standalone task (the description is required).
  import { untrack } from "svelte";
  import type { AssignArgs } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import { TODO_PRIORITIES } from "$lib/todo-list";
  import Modal from "./Modal.svelte";
  import LinkControl from "$lib/controls/LinkControl.svelte";
  import DateControl from "$lib/controls/DateControl.svelte";

  let {
    title,
    icon,
    required,
    assignee = "",
    submitLabel,
    busyLabel,
    onsubmit,
    onclose,
  }: {
    title: string;
    icon: string;
    required: "assignee" | "description";
    /** the user the form starts on */
    assignee?: string;
    submitLabel: string;
    busyLabel: string;
    onsubmit: (args: AssignArgs) => Promise<void>;
    onclose: () => void;
  } = $props();

  let allocatedTo = $state(untrack(() => assignee));
  let date = $state("");
  let priority = $state<(typeof TODO_PRIORITIES)[number]>("Medium");
  let description = $state("");
  let saving = $state(false);

  const userField = { fieldname: "allocated_to", fieldtype: "Link", options: "User", label: "User" };
  const dateField = { fieldname: "date", fieldtype: "Date", label: "Due Date" };

  const missing = $derived(required === "assignee" ? !allocatedTo : !description.trim());

  async function submit() {
    if (missing) return;
    saving = true;
    try {
      await onsubmit({
        allocated_to: allocatedTo,
        date: date || undefined,
        priority,
        description: description.trim() || undefined,
      });
      onclose();
    } catch (e) {
      showError(e);
    } finally {
      saving = false;
    }
  }
</script>

<Modal {title} {icon} size="sm" {onclose}>
  <div class="body">
    <div class="form-group">
      <label for="assignee-input" class="label" class:reqd={required === "assignee"}>{__("Assign to")}</label>
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
        <DateControl id="assign-due-date" field={dateField} value={date} onchange={(v) => (date = v || "")} />
      </div>
      <div class="form-group" style="flex:1">
        <label for="assign-priority" class="label">{__("Priority")}</label>
        <select id="assign-priority" class="input" bind:value={priority}>
          {#each TODO_PRIORITIES as p}<option value={p}>{__(p)}</option>{/each}
        </select>
      </div>
    </div>

    <div class="form-group" style="margin-top:12px">
      <label for="assign-description" class="label" class:reqd={required === "description"}>{__("Description")}</label>
      <textarea
        id="assign-description"
        class="input"
        rows="3"
        placeholder={required === "description" ? __("What needs to be done?") : __("Task instructions / notes")}
        bind:value={description}
      ></textarea>
    </div>
  </div>
  <div class="foot">
    <button class="btn" onclick={onclose}>{__("Cancel")}</button>
    <button class="btn primary" disabled={missing || saving} onclick={submit}>
      {saving ? busyLabel : submitLabel}
    </button>
  </div>
</Modal>
