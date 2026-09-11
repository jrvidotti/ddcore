<script lang="ts">
  import { boot, __, isLoggedIn } from "$lib/boot.svelte";
  import { openShortcutsHelp } from "$lib/shortcuts.svelte";
  import Icon from "./Icon.svelte";
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { goto } from "$app/navigation";
  import { systemDoctypes } from "./sidebar";

  let { open = $bindable(true) }: { open?: boolean } = $props();
  const workspaces = $derived(boot.data?.workspaces || []);
  const current = $derived(page.url.pathname);
  const active = (href: string) => current === href || current.startsWith(href + "/") || current.startsWith(href + "?");
  const itemHref = (it: any) => it.route || (it.doctype ? `/app/${encodeURIComponent(it.doctype)}` : it.report ? `/app/report/${encodeURIComponent(it.report)}` : "");
  async function logout() { await api.logout(); location.href = "/login"; }
  const otherDoctypes = $derived(systemDoctypes(boot.data));
  let showCore = $state(false);

  let menuOpen = $state(false);
  const displayName = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");
  const avatarInitial = (name: string) => (name || "U").trim().charAt(0).toUpperCase();

  // Same close-on-outside-click contract FormView's dropdown uses: a click
  // inside .dropdown is the menu's own business, anything else closes it.
  function onPointerDown(e: PointerEvent) {
    if (!menuOpen) return;
    if ((e.target as HTMLElement | null)?.closest(".dropdown")) return;
    menuOpen = false;
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") menuOpen = false;
  }
</script>

<svelte:window onpointerdown={onPointerDown} onkeydown={onKeydown} />

<aside class="sidebar" class:open>
  <div class="brand">
    <a href="/app" style="display:flex;align-items:center;gap:8px;color:inherit;text-decoration:none"><span class="logo">d</span><strong>{boot.data?.site?.name || "ddcore"}</strong></a>
  </div>
  <nav>
    {#each workspaces as ws}
      {#if workspaces.length > 1}<div class="group">{ws.label || ws.name}</div>{/if}
      {#each ws.sidebar || [] as it}
        {#if itemHref(it)}
          <a href={itemHref(it)} class:active={active(itemHref(it))} class:child={!it.icon}><Icon name={it.icon || "circle"} size={it.icon ? 16 : 6} /><span>{it.label}</span></a>
        {:else}
          <div class="group">{it.label}</div>
        {/if}
      {/each}
    {/each}
    {#if otherDoctypes.length}
      <div class="group" style="cursor:pointer" onclick={() => (showCore = !showCore)} role="button" tabindex="0" onkeydown={(e) => e.key === "Enter" && (showCore = !showCore)}>
        {__("System")} <Icon name={showCore ? "chevron-down" : "chevron-right"} size={12} />
      </div>
      {#if showCore}
        {#each otherDoctypes as [name, d]}
          <a href={`/app/${encodeURIComponent(name)}`} class:active={active(`/app/${encodeURIComponent(name)}`)}><Icon name={d.icon || "circle"} size={d.icon ? 16 : 6} /><span>{d.label}</span></a>
        {/each}
      {/if}
    {/if}
  </nav>
  <div class="foot">
    <div class="dropdown" style="width:100%">
      <button class="user-btn" onclick={() => (menuOpen = !menuOpen)} aria-haspopup="menu" aria-expanded={menuOpen}>
        <span class="avatar">{avatarInitial(displayName)}</span>
        <span class="name small">{displayName}</span>
        <Icon name={menuOpen ? "chevron-down" : "chevron-right"} size={14} />
      </button>
      {#if menuOpen}
        <div class="menu up" role="menu">
          <button role="menuitem" onclick={() => { menuOpen = false; goto("/app/profile"); }}>
            <Icon name="user" size={14} /> {__("My profile")}
          </button>
          <button role="menuitem" onclick={() => { menuOpen = false; openShortcutsHelp(); }}>
            <Icon name="keyboard" size={14} /> {__("Keyboard shortcuts")}
          </button>
          <button role="menuitem" onclick={logout}>
            <Icon name="log-out" size={14} /> {__("Sign out")}
          </button>
        </div>
      {/if}
    </div>
  </div>
</aside>

<style>
  .sidebar { width: var(--sidebar-w); background: #fff; border-right: 1px solid var(--border); display: flex; flex-direction: column; height: 100vh; position: sticky; top: 0; flex-shrink: 0; }
  .brand { padding: 14px 16px; border-bottom: 1px solid var(--border); font-size: 15px; }
  .logo { display: inline-flex; width: 26px; height: 26px; border-radius: 7px; background: var(--primary); color: #fff; align-items: center; justify-content: center; font-weight: 700; }
  nav { flex: 1; overflow: auto; padding: 8px; }
  nav a { display: flex; align-items: center; gap: 10px; padding: 7px 10px; border-radius: 6px; color: var(--text); font-size: 13px; }
  nav a:hover { background: #f3f4f6; text-decoration: none; }
  nav a.active { background: #eff6ff; color: var(--primary); font-weight: 500; }
  nav a.child { padding-left: 16px; }
  .group { font-size: 11px; text-transform: uppercase; letter-spacing: .05em; color: var(--muted); padding: 12px 10px 4px; display: flex; align-items: center; gap: 4px; }
  .foot { padding: 8px 10px; border-top: 1px solid var(--border); }
  .user-btn { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; border: 0; background: none; border-radius: 6px; cursor: pointer; text-align: left; color: inherit; font: inherit; }
  .user-btn:hover { background: #f3f4f6; }
  .user-btn .name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .avatar { display: inline-flex; width: 24px; height: 24px; flex-shrink: 0; border-radius: 50%; background: var(--primary); color: #fff; align-items: center; justify-content: center; font-size: 11px; font-weight: 600; }
  /* The footer sits at the bottom of the viewport, so the menu opens upward. */
  :global(.dropdown .menu.up) { top: auto; bottom: 100%; margin: 0 0 4px; left: 0; right: 0; }
  :global(.dropdown .menu.up button) { display: flex; align-items: center; gap: 8px; }
  @media (max-width: 800px) { .sidebar { position: fixed; z-index: 50; transform: translateX(-100%); transition: transform .15s; } .sidebar.open { transform: none; } }
</style>
