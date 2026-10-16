<script lang="ts">
  // Plays back one recorded audio note; the object URL lives as long as the file is shown.
  let { file }: { file: File } = $props();

  let url = $state("");
  $effect(() => {
    const f = file;
    if (!f || typeof URL.createObjectURL !== "function") { url = ""; return; }
    const u = URL.createObjectURL(f);
    url = u;
    return () => URL.revokeObjectURL(u);
  });
</script>

{#if url}<audio controls src={url}></audio>{:else}<span class="small">{file.name}</span>{/if}

<style>
  audio { height: 32px; max-width: 100%; flex: 1 1 200px; min-width: 0; }
</style>
