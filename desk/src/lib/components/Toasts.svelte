<script lang="ts">
  import { ui } from "$lib/ui.svelte";
</script>

<div class="toasts">
  {#each ui.toasts as t (t.id)}
    <div class="toast {t.indicator || 'blue'}" role="status">
      {#if t.title}<div class="t">{t.title}</div>{/if}
      <div>{@html t.message}</div>
    </div>
  {/each}
</div>
