<script lang="ts">
  import SiteMark from "$lib/components/SiteMark.svelte";
  // The portal's own chrome (OPS-10): the site's name, the pages of the
  // portals the user reaches, and their account. Nothing of the desk.
  import { __, boot, siteName } from "$lib/boot.svelte";
  import { signOut } from "$lib/sign-out";
  import { stopImpersonating } from "$lib/impersonation";
  import { page } from "$app/state";
  import Icon from "$lib/components/Icon.svelte";
  import ImpersonationBanner from "$lib/components/ImpersonationBanner.svelte";
  import { portalHref } from "$lib/portal";
  import { dismiss } from "$lib/controls/floating";

  let { children } = $props();
  let menuOpen = $state(false);

  const portals = $derived(boot.data?.portals || []);
  const name = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");

  function active(slug: string, pageName: string) {
    return page.url.pathname === portalHref(slug, pageName) || page.url.pathname.startsWith(portalHref(slug, pageName) + "/");
  }

</script>

<div class="portal">
  <ImpersonationBanner />
  <header>
    <a class="brand" href="/portal"><SiteMark /><span class="site-name">{siteName()}</span></a>
    <nav>
      {#each portals as p (p.slug)}
        {#each p.pages as pg (pg.name)}
          <a href={portalHref(p.slug, pg.name)} class:active={active(p.slug, pg.name)}>{pg.label}</a>
        {/each}
      {/each}
    </nav>
    <div class="account" use:dismiss={menuOpen ? () => (menuOpen = false) : null}>
      <button class="btn" type="button" onclick={() => (menuOpen = !menuOpen)} aria-haspopup="menu" aria-expanded={menuOpen}>
        <Icon name="user" size={14} /> <span class="who">{name}</span>
      </button>
      {#if menuOpen}
        <div class="menu" role="menu">
          <a role="menuitem" href="/portal/profile" onclick={() => (menuOpen = false)}>{__("My profile")}</a>
          {#if !boot.data?.website}
            <a role="menuitem" href="/app">{__("Back to the desk")}</a>
          {/if}
          {#if boot.data?.impersonatedBy}
            <button role="menuitem" type="button" onclick={stopImpersonating}>{__("Stop impersonating")}</button>
          {/if}
          <button role="menuitem" type="button" onclick={signOut}>{__("Sign out")}</button>
        </div>
      {/if}
    </div>
  </header>
  <main>{@render children()}</main>
</div>

<style>
  .portal { min-height: 100vh; background: var(--bg); }
  header {
    display: flex; align-items: center; gap: 16px; padding: 0 20px; min-height: 56px;
    background: var(--surface); border-bottom: 1px solid var(--border); flex-wrap: wrap;
  }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 600; color: inherit; text-decoration: none; }
  nav { display: flex; gap: 4px; flex: 1; overflow-x: auto; }
  nav a { padding: 6px 10px; border-radius: 6px; color: var(--muted); text-decoration: none; white-space: nowrap; font-size: 14px; }
  nav a:hover { background: var(--hover); text-decoration: none; }
  nav a.active { color: var(--text); background: var(--hover-strong); font-weight: 500; }
  .account { position: relative; }
  .who { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .menu {
    position: absolute; right: 0; top: calc(100% + 6px); min-width: 180px; background: var(--surface); border: 1px solid var(--border);
    border-radius: 8px; box-shadow: 0 10px 30px rgba(0,0,0,.12); padding: 4px; z-index: 60;
  }
  .menu a, .menu button {
    display: block; width: 100%; text-align: left; padding: 8px 10px; border: 0; background: none; border-radius: 5px;
    font-size: 13px; color: var(--text); cursor: pointer; text-decoration: none;
  }
  .menu a:hover, .menu button:hover { background: var(--hover); }
  main { padding: 20px 16px 40px; }
  @media (max-width: 800px) { .who { display: none; } header { padding: 8px 12px; } }
</style>
