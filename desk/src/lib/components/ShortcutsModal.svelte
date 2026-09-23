<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  import { shortcutsState, closeShortcutsHelp, getShortcutsList } from "$lib/shortcuts.svelte";
  import Icon from "./Icon.svelte";
  import { __ } from "$lib/boot.svelte";

  const groups = $derived(getShortcutsList());
</script>

{#if shortcutsState.open}
  <div
    class="modal-bg"
    use:focusTrap
    role="dialog"
    aria-modal="true"
    aria-labelledby="shortcuts-title"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && closeShortcutsHelp()}
    onkeydown={(e) => e.key === "Escape" && closeShortcutsHelp()}
  >
    <div class="modal md">
      <div class="head">
        <h3 id="shortcuts-title" style="display:flex;align-items:center;gap:8px">
          <Icon name="keyboard" size={18} />
          {__("Keyboard shortcuts")}
        </h3>
        <button class="btn icon" onclick={closeShortcutsHelp} aria-label={__("Close")}><Icon name="x" /></button>
      </div>
      <div class="body">
        <div class="shortcuts-grid">
          {#each groups as group}
            <div>
              <h4 class="shortcut-group-title">{__(group.category)}</h4>
              <div class="shortcut-list">
                {#each group.shortcuts as item}
                  <div class="shortcut-row">
                    <span>{__(item.description)}</span>
                    <span class="shortcut-keys">
                      {#each item.keys as k, i}
                        <kbd class="kbd">{k}</kbd>{#if i < item.keys.length - 1}<span class="kbd-plus">+</span>{/if}
                      {/each}
                    </span>
                  </div>
                {/each}
              </div>
            </div>
          {/each}
        </div>
      </div>
      <div class="foot">
        <button class="btn" onclick={closeShortcutsHelp}>{__("Close")}</button>
      </div>
    </div>
  </div>
{/if}
