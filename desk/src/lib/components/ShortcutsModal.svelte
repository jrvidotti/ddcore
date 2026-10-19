<script lang="ts">
  import { shortcutsState, closeShortcutsHelp, getShortcutsList } from "$lib/shortcuts.svelte";
  import Modal from "./Modal.svelte";
  import { __ } from "$lib/boot.svelte";

  const groups = $derived(getShortcutsList());
</script>

{#if shortcutsState.open}
  <Modal title={__("Keyboard shortcuts")} icon="keyboard" onclose={closeShortcutsHelp}>
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
  </Modal>
{/if}
