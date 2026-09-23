<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  import type { ShareArgs } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";
  import LinkControl from "$lib/controls/LinkControl.svelte";

  let {
    open = false,
    canOverrideScope = false,
    onshare,
    onclose,
  }: {
    open: boolean;
    canOverrideScope: boolean;
    onshare: (args: ShareArgs) => Promise<void>;
    onclose: () => void;
  } = $props();

  let user = $state("");
  let write = $state(false);
  let share = $state(false);
  let overrideScope = $state(false);
  let saving = $state(false);

  const userField = { fieldname: "user", fieldtype: "Link", options: "User", label: "User" };

  async function submit() {
    if (!user) return;
    saving = true;
    try {
      await onshare({ user, write, share, overrideScope: canOverrideScope && overrideScope });
      onclose();
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
    use:focusTrap
    role="dialog"
    aria-modal="true"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && onclose()}
    onkeydown={(e) => e.key === "Escape" && onclose()}
  >
    <div class="modal sm">
      <div class="head">
        <h3 style="display:flex;align-items:center;gap:8px">
          <Icon name="share-2" size={18} />
          {__("Share document")}
        </h3>
        <button class="btn icon" onclick={onclose} aria-label={__("Close")}>
          <Icon name="x" size={16} />
        </button>
      </div>
      <div class="body">
        <div class="form-group">
          <label for="share-user-input" class="label reqd">{__("Share with")}</label>
          <LinkControl id="share-user-input" field={userField} value={user} onchange={(v) => (user = v || "")} />
        </div>

        <fieldset class="rights">
          <legend class="label">{__("Rights")}</legend>
          <label class="check"><input type="checkbox" checked disabled /> {__("Can Read")}</label>
          <label class="check"><input type="checkbox" bind:checked={write} /> {__("Can Write")}</label>
          <label class="check"><input type="checkbox" bind:checked={share} /> {__("Can Share")}</label>
        </fieldset>

        {#if canOverrideScope}
          <label class="check override">
            <input type="checkbox" bind:checked={overrideScope} />
            <span>
              {__("Override Security Scope")}
              <span class="small muted hint">{__("Reach the user even when the document is outside their User Permission scope.")}</span>
            </span>
          </label>
        {/if}
      </div>
      <div class="foot">
        <button class="btn" onclick={onclose}>{__("Cancel")}</button>
        <button class="btn primary" disabled={!user || saving} onclick={submit}>
          {saving ? __("Sharing...") : __("Share")}
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
  .rights {
    display: flex;
    gap: 16px;
    flex-wrap: wrap;
    border: 0;
    padding: 0;
    margin: 12px 0 0;
  }
  .rights legend {
    width: 100%;
  }
  .check {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 13px;
  }
  .override {
    margin-top: 14px;
  }
  .hint {
    display: block;
    margin-top: 2px;
  }
</style>
