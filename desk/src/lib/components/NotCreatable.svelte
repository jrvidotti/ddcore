<script lang="ts">
  // Drawn in place of a new form the reader cannot create: a DocType only
  // server code creates (`allowCreate: false`), or a role without `create`.
  // The form would take the typing and the server would refuse the save.
  import { __ } from "$lib/boot.svelte";

  let { label, description = "", href }: { label: string; description?: string; href: string } = $props();
</script>

<div class="page">
  <div class="page-head"><h1>{label}</h1></div>
  <div class="card empty not-creatable" role="alert">
    <p>{__("{0} is not created here", [label])}</p>
    {#if description}<p class="muted">{description}</p>{/if}
    <div class="actions"><a class="btn" {href}>{__("Back to {0}", [label])}</a></div>
  </div>
</div>

<style>
  .not-creatable p { margin: 0 0 12px; }
  .actions { display: flex; justify-content: center; margin-top: 16px; }
</style>
