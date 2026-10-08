<script lang="ts">
  // A grid's actions on its selected rows (see frm.addGridAction), in its
  // toolbar: each reads "label (n)" and gets only the rows it applies to.
  import type { GridAction } from "$lib/form.svelte";
  import { showError } from "$lib/ui.svelte";
  import { visibleGridActions } from "$lib/components/list-actions";

  let { actions = [], rows = [], ondone }: {
    actions?: GridAction[];
    /** the selected rows on screen, in screen order */
    rows?: any[];
    /** called once an action settles, to clear the selection */
    ondone: () => void;
  } = $props();
  const visible = $derived(visibleGridActions(actions, rows));
  let running = $state(false);

  async function run(action: GridAction, actionRows: any[]) {
    running = true;
    try {
      await action.onClick(actionRows);
    } catch (e) { showError(e); }
    finally { running = false; ondone(); }
  }
</script>

{#each visible as { action, rows: actionRows } (action)}
  <button type="button" class="btn sm grid-action" class:primary={action.primary} disabled={running} onclick={() => run(action, actionRows)}>{action.label} ({actionRows.length})</button>
{/each}
