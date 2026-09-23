<script lang="ts">
  // The portal's start: every page of every portal the user reaches.
  import { __, boot } from "$lib/boot.svelte";
  import { portalHref } from "$lib/portal";

  const portals = $derived(boot.data?.portals || []);
</script>

<div class="page start">
  {#if !portals.length}
    <div class="card empty">{__("Your account has no portal yet. Ask the people who invited you.")}</div>
  {:else}
    {#each portals as p (p.slug)}
      <h1>{p.title}</h1>
      <div class="cards">
        {#each p.pages as pg (pg.name)}
          <a class="card tile" href={portalHref(p.slug, pg.name)}>
            <strong>{pg.label}</strong>
            {#if pg.description}<span class="muted small">{pg.description}</span>{/if}
          </a>
        {/each}
      </div>
    {/each}
  {/if}
</div>

<style>
  .start { max-width: 960px; margin: 0 auto; }
  h1 { margin: 8px 0 16px; }
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(220px, 1fr)); gap: 12px; margin-bottom: 24px; }
  .tile { display: flex; flex-direction: column; gap: 4px; padding: 16px 18px; color: inherit; text-decoration: none; }
  .tile:hover { border-color: var(--primary); text-decoration: none; }
  .empty { padding: 24px; text-align: center; color: var(--muted); }
</style>
