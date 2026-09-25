<script lang="ts">
  import { __ } from "$lib/boot.svelte";

  // full: fill the viewport and centre the ring (the desk's boot screen).
  let { size = 24, full = false }: { size?: number; full?: boolean } = $props();
</script>

<div class="spinner-wrap" class:full role="status">
  <span class="ring" style:width="{size}px" style:height="{size}px"></span>
  <span class="label">{__("Loading…")}</span>
</div>

<style>
  .spinner-wrap { display: flex; align-items: center; justify-content: center; padding: 40px 0; }
  .spinner-wrap.full { min-height: 100vh; padding: 0; }
  .ring {
    box-sizing: border-box;
    border: 3px solid var(--border, rgba(0, 0, 0, 0.1));
    border-top-color: var(--primary);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  .label {
    position: absolute; width: 1px; height: 1px; overflow: hidden;
    clip: rect(0 0 0 0); clip-path: inset(50%); white-space: nowrap;
  }
  @keyframes spin { to { transform: rotate(360deg); } }
  @media (prefers-reduced-motion: reduce) { .ring { animation-duration: 2.4s; } }
</style>
