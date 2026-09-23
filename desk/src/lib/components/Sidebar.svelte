<script lang="ts">
  import { boot, __, isLoggedIn, siteName, siteLogo } from "$lib/boot.svelte";
  import { openShortcutsHelp, openSearch } from "$lib/shortcuts.svelte";
  import { getModifierKey } from "$lib/shortcuts";
  import Icon from "./Icon.svelte";
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { goto } from "$app/navigation";
  import { notifications, stopNotifications } from "$lib/notifications.svelte";
  import { pendingTasks } from "$lib/assignments.svelte";
  import { disconnectEvents } from "$lib/events";
  import { isActiveLink, systemDoctypes } from "./sidebar";
  import {
    resolveActiveWorkspace,
    rememberWorkspace,
    getRememberedWorkspace,
    workspaceItemHref,
    type WorkspaceItem,
  } from "./sidebar-workspace";

  let { open = $bindable(true) }: { open?: boolean } = $props();
  const workspaces = $derived((boot.data?.workspaces || []) as WorkspaceItem[]);
  const current = $derived(page.url.pathname);
  const active = (href: string) => isActiveLink(current, href, workspaces.map((w) => w.name));

  let remembered = $state(getRememberedWorkspace());
  const activeWorkspace = $derived(resolveActiveWorkspace({
    currentPath: current,
    workspaces,
    doctypes: boot.data?.doctypes,
    remembered,
  }));

  $effect(() => {
    if (activeWorkspace?.name && activeWorkspace.name !== remembered) {
      remembered = activeWorkspace.name;
      rememberWorkspace(activeWorkspace.name);
    }
  });

  let wsMenuOpen = $state(false);

  function selectWorkspace(name: string) {
    wsMenuOpen = false;
    rememberWorkspace(name);
    remembered = name;
    if (typeof window !== "undefined" && window.innerWidth <= 800) {
      open = false;
    }
    goto(`/app/${encodeURIComponent(name)}`);
  }

  function onWindowClick(e: MouseEvent) {
    if (!open || typeof window === "undefined" || window.innerWidth > 800) return;
    const target = e.target as HTMLElement | null;
    if (target?.closest("aside.sidebar a")) {
      open = false;
    }
  }

  const itemHref = (it: any) => workspaceItemHref(activeWorkspace?.name || "", it);
  async function logout() {
    if (typeof window !== "undefined" && window.innerWidth <= 800) open = false;
    await api.logout();
    stopNotifications();
    disconnectEvents();
    location.href = "/login";
  }
  const otherDoctypes = $derived(systemDoctypes(boot.data));
  let showCore = $state(false);

  let menuOpen = $state(false);
  const displayName = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");
  const avatarInitial = (name: string) => (name || "U").trim().charAt(0).toUpperCase();

  function onPointerDown(e: PointerEvent) {
    const target = e.target as HTMLElement | null;
    if (menuOpen && !target?.closest(".foot .dropdown")) menuOpen = false;
    if (wsMenuOpen && !target?.closest(".workspace-switcher")) wsMenuOpen = false;
  }
  function onKeydown(e: KeyboardEvent) {
    if (e.key === "Escape") {
      menuOpen = false;
      wsMenuOpen = false;
    }
  }
</script>

<svelte:window onpointerdown={onPointerDown} onkeydown={onKeydown} onclick={onWindowClick} />

<aside class="sidebar" class:open>
  <div class="brand">
    <a href="/app" style="display:flex;align-items:center;gap:8px;color:inherit;text-decoration:none"><span class="logo">{siteLogo()}</span><strong>{siteName()}</strong></a>
    <button class="btn icon sidebar-close-btn" onclick={() => (open = false)} aria-label={__("Close menu")} type="button">
      <Icon name="x" size={16} />
    </button>
  </div>
  {#if workspaces.length > 1}
    <div class="workspace-switcher dropdown">
      <button class="workspace-btn" onclick={() => (wsMenuOpen = !wsMenuOpen)} aria-haspopup="menu" aria-expanded={wsMenuOpen}>
        <span class="ws-icon"><Icon name={activeWorkspace?.icon || "layout-dashboard"} size={16} /></span>
        <span class="ws-label">{activeWorkspace?.label || activeWorkspace?.name}</span>
        <Icon name={wsMenuOpen ? "chevron-up" : "chevron-down"} size={13} />
      </button>
      {#if wsMenuOpen}
        <div class="menu" role="menu">
          {#each workspaces as ws}
            <button
              role="menuitem"
              class:selected={ws.name === activeWorkspace?.name}
              onclick={() => selectWorkspace(ws.name)}
            >
              <Icon name={ws.icon || "layout-dashboard"} size={14} />
              <span class="ws-option-label">{ws.label || ws.name}</span>
              {#if ws.name === activeWorkspace?.name}
                <Icon name="check" size={14} />
              {/if}
            </button>
          {/each}
        </div>
      {/if}
    </div>
  {:else if workspaces.length === 1}
    <div class="workspace-static-header">
      <Icon name={workspaces[0].icon || "layout-dashboard"} size={16} />
      <span>{workspaces[0].label || workspaces[0].name}</span>
    </div>
  {/if}
  <nav>
    <button class="search-btn" type="button" onclick={() => { if (typeof window !== "undefined" && window.innerWidth <= 800) open = false; openSearch(); }}>
      <Icon name="search" /><span>{__("Search")}</span><kbd class="kbd">{getModifierKey()} K</kbd>
    </button>
    <a href="/app/notifications" class:active={active("/app/notifications")}>
      <Icon name="bell" /><span>{__("Notifications")}</span>
      {#if notifications.unread > 0}<span class="notification-count" aria-label={__("{0} unread notifications", [notifications.unread])}>{notifications.unread}</span>{/if}
    </a>
    <a href="/app/todo" class:active={active("/app/todo")}>
      <Icon name="check-square" /><span>{__("To-Do")}</span>
      {#if pendingTasks.count > 0}<span class="notification-count" aria-label={__("{0} pending tasks", [pendingTasks.count])}>{pendingTasks.count}</span>{/if}
    </a>
    {#if activeWorkspace}
      {#each activeWorkspace.sidebar || [] as it}
        {#if itemHref(it)}
          <a href={itemHref(it)} class:active={active(itemHref(it))} class:child={!it.icon}><Icon name={it.icon || "circle"} size={it.icon ? 16 : 6} /><span>{it.label}</span></a>
        {:else}
          <div class="group">{it.label}</div>
        {/if}
      {/each}
    {/if}
    {#if otherDoctypes.length}
      <div class="group" style="cursor:pointer" onclick={() => (showCore = !showCore)} role="button" tabindex="0" onkeydown={(e) => e.key === "Enter" && (showCore = !showCore)}>
        {__("System")} <Icon name={showCore ? "chevron-down" : "chevron-right"} size={12} />
      </div>
      {#if showCore}
        {#each otherDoctypes as [name, d]}
          {@const coreHref = `/app/${encodeURIComponent(activeWorkspace?.name || "core")}/${encodeURIComponent(name)}`}
          <a href={coreHref} class:active={active(coreHref)}><Icon name={d.icon || "circle"} size={d.icon ? 16 : 6} /><span>{d.label}</span></a>
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
          <button role="menuitem" onclick={() => { menuOpen = false; if (typeof window !== "undefined" && window.innerWidth <= 800) open = false; goto("/app/profile"); }}>
            <Icon name="user" size={14} /> {__("My profile")}
          </button>
          {#if boot.data?.portals?.length}
            <!-- a desk user who is also, say, an employee reaches their own portal (OPS-10) -->
            <button role="menuitem" onclick={() => { menuOpen = false; goto("/portal"); }}>
              <Icon name="external-link" size={14} /> {__("My portal")}
            </button>
          {/if}
          <button role="menuitem" onclick={() => { menuOpen = false; if (typeof window !== "undefined" && window.innerWidth <= 800) open = false; openShortcutsHelp(); }}>
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
  .logo { display: inline-flex; width: 26px; height: 26px; border-radius: 7px; background: var(--primary); color: #fff; align-items: center; justify-content: center; font-weight: 700; overflow: hidden; }
  .sidebar-close-btn { display: none; }
  .workspace-switcher { padding: 8px 10px; border-bottom: 1px solid var(--border); }
  .workspace-btn { display: flex; align-items: center; gap: 8px; width: 100%; padding: 6px 8px; border: 1px solid var(--border); background: #fafafa; border-radius: 6px; cursor: pointer; text-align: left; font-size: 13px; font-weight: 500; color: var(--text); }
  .workspace-btn:hover { background: #f3f4f6; }
  .workspace-btn .ws-icon { display: flex; align-items: center; color: var(--primary); }
  .workspace-btn .ws-label { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .workspace-switcher .menu { width: calc(100% - 20px); left: 10px; right: 10px; }
  .workspace-switcher .menu button { display: flex; align-items: center; gap: 8px; }
  .workspace-switcher .menu button.selected { background: #eff6ff; color: var(--primary); font-weight: 500; }
  .workspace-switcher .menu .ws-option-label { flex: 1; text-align: left; }
  .workspace-static-header { display: flex; align-items: center; gap: 8px; padding: 10px 14px; border-bottom: 1px solid var(--border); font-size: 13px; font-weight: 600; color: var(--text); }
  nav { flex: 1; overflow: auto; padding: 8px; }
  nav a { display: flex; align-items: center; gap: 10px; padding: 7px 10px; border-radius: 6px; color: var(--text); font-size: 13px; }
  .search-btn { display: flex; align-items: center; gap: 10px; width: 100%; padding: 7px 10px; margin-bottom: 4px; border: 1px solid var(--border); border-radius: 6px; background: #fafafa; color: var(--muted); font-size: 13px; cursor: pointer; text-align: left; }
  .search-btn span { flex: 1; }
  .search-btn:hover { background: #f3f4f6; }
  .search-btn .kbd { font-size: 11px; }
  nav a:hover { background: #f3f4f6; text-decoration: none; }
  nav a.active { background: #eff6ff; color: var(--primary); font-weight: 500; }
  .notification-count { margin-left: auto; border-radius: 12px; padding: 1px 7px; background: var(--primary); color: white; font-size: 11px; }
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
  @media (max-width: 800px) {
    .brand { display: flex; align-items: center; justify-content: space-between; }
    .sidebar-close-btn { display: inline-flex; width: 28px; height: 28px; padding: 0; align-items: center; justify-content: center; border: 0; background: none; color: var(--muted); cursor: pointer; border-radius: 4px; }
    .sidebar-close-btn:hover { background: #f3f4f6; color: var(--text); }
    .sidebar { position: fixed; z-index: 50; transform: translateX(-100%); transition: transform .2s ease; box-shadow: none; }
    .sidebar.open { transform: none; box-shadow: 4px 0 24px rgba(0, 0, 0, 0.15); }
  }
</style>
