<script lang="ts">
  // The portal's own chrome (OPS-10): the site's name, the pages of the
  // portals the user reaches, and their account. Nothing of the desk.
  import { __, boot, siteLogo, siteName } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { page } from "$app/state";
  import Icon from "$lib/components/Icon.svelte";
  import { portalHref } from "$lib/portal";

  let { children } = $props();
  let menuOpen = $state(false);

  const portals = $derived(boot.data?.portals || []);
  const name = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");

  function active(slug: string, pageName: string) {
    return page.url.pathname === portalHref(slug, pageName) || page.url.pathname.startsWith(portalHref(slug, pageName) + "/");
  }

  async function signOut() {
    await api.logout();
    location.href = "/login";
  }
</script>

<svelte:window onpointerdown={(e) => { if (menuOpen && !(e.target as HTMLElement)?.closest(".account")) menuOpen = false; }} />

<div class="portal">
  <header>
    <a class="brand" href="/portal"><span class="logo">{siteLogo()}</span><span>{siteName()}</span></a>
    <nav>
      {#each portals as p (p.slug)}
        {#each p.pages as pg (pg.name)}
          <a href={portalHref(p.slug, pg.name)} class:active={active(p.slug, pg.name)}>{pg.label}</a>
        {/each}
      {/each}
    </nav>
    <div class="account">
      <button class="btn" type="button" onclick={() => (menuOpen = !menuOpen)} aria-haspopup="menu" aria-expanded={menuOpen}>
        <Icon name="user" size={14} /> <span class="who">{name}</span>
      </button>
      {#if menuOpen}
        <div class="menu" role="menu">
          <a role="menuitem" href="/portal/profile" onclick={() => (menuOpen = false)}>{__("My profile")}</a>
          {#if !boot.data?.website}
            <a role="menuitem" href="/app">{__("Back to the desk")}</a>
          {/if}
          <button role="menuitem" type="button" onclick={signOut}>{__("Sign out")}</button>
        </div>
      {/if}
    </div>
  </header>
  <main>{@render children()}</main>
</div>

<style>
  .portal { min-height: 100vh; background: var(--bg, #f7f7f8); }
  header {
    display: flex; align-items: center; gap: 16px; padding: 0 20px; min-height: 56px;
    background: #fff; border-bottom: 1px solid var(--border); flex-wrap: wrap;
  }
  .brand { display: flex; align-items: center; gap: 8px; font-weight: 600; color: inherit; text-decoration: none; }
  .logo {
    display: inline-flex; width: 26px; height: 26px; border-radius: 6px; background: var(--primary); color: #fff;
    align-items: center; justify-content: center; font-size: 13px; font-weight: 700;
  }
  nav { display: flex; gap: 4px; flex: 1; overflow-x: auto; }
  nav a { padding: 6px 10px; border-radius: 6px; color: var(--muted); text-decoration: none; white-space: nowrap; font-size: 14px; }
  nav a:hover { background: #f3f4f6; text-decoration: none; }
  nav a.active { color: var(--text); background: #eef0f3; font-weight: 500; }
  .account { position: relative; }
  .who { max-width: 180px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .menu {
    position: absolute; right: 0; top: calc(100% + 6px); min-width: 180px; background: #fff; border: 1px solid var(--border);
    border-radius: 8px; box-shadow: 0 10px 30px rgba(0,0,0,.12); padding: 4px; z-index: 60;
  }
  .menu a, .menu button {
    display: block; width: 100%; text-align: left; padding: 8px 10px; border: 0; background: none; border-radius: 5px;
    font-size: 13px; color: var(--text); cursor: pointer; text-decoration: none;
  }
  .menu a:hover, .menu button:hover { background: #f3f4f6; }
  main { padding: 20px 16px 40px; }
  @media (max-width: 800px) { .who { display: none; } header { padding: 8px 12px; } }
</style>
