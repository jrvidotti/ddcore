<script lang="ts">
  import "../app.css";
  import { __, loadBoot, isLoggedIn, siteName, siteLogo, boot, type Boot } from "$lib/boot.svelte";
  import { installDeskSDK, loadAppIncludes } from "$lib/desk-sdk";
  import { connectEvents, disconnectEvents, subscribe } from "$lib/events";
  import { maintenance, setMaintenance } from "$lib/maintenance.svelte";
  import { startNotifications, stopNotifications, notifications } from "$lib/notifications.svelte";
  import { startPendingTasks, stopPendingTasks } from "$lib/assignments.svelte";
  import { clearMetaCache } from "$lib/meta";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import Toasts from "$lib/components/Toasts.svelte";
  import Dialogs from "$lib/components/Dialogs.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import ShortcutsModal from "$lib/components/ShortcutsModal.svelte";
  import SearchPalette from "$lib/components/SearchPalette.svelte";
  import { shouldOpenSearch } from "$lib/components/search-palette";
  import { ui, toast } from "$lib/ui.svelte";
  import { shouldToggleShortcuts, toggleShortcutsHelp, shortcutsState, closeShortcutsHelp, openShortcutsHelp, searchState, openSearch, closeSearch } from "$lib/shortcuts.svelte";
  import { api, onMessage } from "$lib/api";
  import { isPortalPath, portalRedirect } from "$lib/portal";
  import { page } from "$app/state";
  import { goto, afterNavigate } from "$app/navigation";
  import { onMount, onDestroy, untrack } from "svelte";
  import {
    resolveActiveWorkspace,
    getRememberedWorkspace,
    type WorkspaceItem,
  } from "$lib/components/sidebar-workspace";

  let { children } = $props();
  let ready = $state(false);
  let sidebarOpen = $state(false);
  let userMenuOpen = $state(false);
  const isLogin = $derived(page.url.pathname.startsWith("/login"));
  // the portal draws its own chrome: none of the desk's shell (OPS-10)
  const isPortal = $derived(isPortalPath(page.url.pathname));

  const displayName = $derived(boot.data?.userDoc?.full_name || boot.data?.user || "");
  const avatarInitial = (name: string) => (name || "U").trim().charAt(0).toUpperCase();

  const activeWorkspaceName = $derived.by(() => {
    const ws = resolveActiveWorkspace({
      currentPath: page.url.pathname,
      workspaces: (boot.data?.workspaces || []) as WorkspaceItem[],
      doctypes: boot.data?.doctypes,
      remembered: getRememberedWorkspace(),
    });
    return ws?.label || ws?.name || "";
  });

  function onWindowKeydown(e: KeyboardEvent) {
    if (isLoggedIn() && !isLogin && !isPortal && shouldOpenSearch(e)) {
      e.preventDefault();
      if (searchState.open) closeSearch(); else { closeShortcutsHelp(); openSearch(); }
      return;
    }
    if (shortcutsState.open && e.key === "Escape") {
      closeShortcutsHelp();
      return;
    }
    if (userMenuOpen && e.key === "Escape") {
      userMenuOpen = false;
      return;
    }
    if (sidebarOpen && e.key === "Escape") {
      sidebarOpen = false;
      return;
    }
    if (shouldToggleShortcuts(e)) {
      e.preventDefault();
      toggleShortcutsHelp();
    }
  }

  function onWindowPointerDown(e: PointerEvent) {
    const target = e.target as HTMLElement | null;
    if (userMenuOpen && !target?.closest(".mobile-user-dropdown")) {
      userMenuOpen = false;
    }
  }

  async function handleLogout() {
    userMenuOpen = false;
    if (typeof window !== "undefined" && window.innerWidth <= 800) {
      sidebarOpen = false;
    }
    await api.logout();
    stopNotifications();
    stopPendingTasks();
    disconnectEvents();
    location.href = "/login";
  }

  // Automatically close sidebar and user menu on mobile when navigating to another route
  afterNavigate(() => {
    userMenuOpen = false;
    if (typeof window !== "undefined" && window.innerWidth <= 800) {
      sidebarOpen = false;
    }
  });

  $effect(() => {
    // Lock document body scroll when mobile drawer is open
    if (typeof document !== "undefined") {
      if (sidebarOpen && window.innerWidth <= 800) {
        document.body.style.overflow = "hidden";
      } else {
        document.body.style.overflow = "";
      }
    }
    return () => {
      if (typeof document !== "undefined") {
        document.body.style.overflow = "";
      }
    };
  });

  // The stop functions read and write store state (`revision++`); untracked, so the effect
  // depends on `isLogin` alone instead of re-running on its own writes.
  $effect(() => {
    if (isLogin) untrack(() => { stopNotifications(); stopPendingTasks(); disconnectEvents(); });
  });
  onDestroy(() => { stopNotifications(); stopPendingTasks(); disconnectEvents(); });
  // a link into the desk followed from the portal lands back in it
  $effect(() => {
    const to = ready ? portalRedirect(boot.data?.website, page.url.pathname) : null;
    if (to) untrack(() => goto(to, { replaceState: true }));
  });

  const deskIncludeApps = (b: Boot) => b.apps.filter((a) => a.hasDeskInclude).map((a) => a.name);
  // a desk user gets the portal's scripts on first entering "My portal", not at
  // boot, so one who never opens it never runs them on desk forms; the effect
  // re-runs on a reloaded boot, which picks up an app that just declared some
  $effect(() => {
    const b = boot.data;
    if (ready && isPortal && b && !b.website && b.user !== "Guest") untrack(() => loadAppIncludes(b.portalIncludes ?? [], b.loaded, "portal"));
  });

  onMount(async () => {
    installDeskSDK();
    onMessage((m) => toast(m.message, { title: m.title, indicator: m.indicator || "blue" }));
    try {
      const b = await loadBoot();
      // app.html ships lang="en"; the boot is what knows the real one
      document.documentElement.lang = b.lang;
      setMaintenance(b.site?.maintenance);
      // Awaited: rendering the page before the URL changes lets its own redirect (/app → a
      // workspace) replace this one, and a guest never reaches the sign-in form.
      if (b.user === "Guest" && !isLogin) { await goto("/login?redirect=" + encodeURIComponent(page.url.pathname + page.url.search)); }
      else if (b.website) {
        // a Website User reaches the portals and nothing else; the desk's
        // includes, notifications and event stream are desk API they are refused
        const to = portalRedirect(b.website, page.url.pathname);
        if (to) await goto(to, { replaceState: true });
        await loadAppIncludes(b.portalIncludes ?? [], b.loaded, "portal");
      }
      else if (b.user !== "Guest") {
        await loadAppIncludes(deskIncludeApps(b), b.loaded, "desk");
        startNotifications();
        startPendingTasks();
        subscribe("maintenance", (p) => setMaintenance(p));
        connectEvents(async () => { clearMetaCache(); const nb = await loadBoot(); await loadAppIncludes(deskIncludeApps(nb), nb.loaded, "desk"); toast(__("Apps reloaded"), { indicator: "blue", timeout: 2000 }); });
      }
    } catch (e) { console.error(e); }
    ready = true;
  });
</script>

<svelte:head><title>{siteName()}</title></svelte:head>
<svelte:window onkeydown={onWindowKeydown} onpointerdown={onWindowPointerDown} />

{#if ui.busy > 0}<div class="busy-bar"></div>{/if}
{#if !ready}
  <Spinner full />
{:else if isLogin || !isLoggedIn() || isPortal}
  {@render children()}
{:else}
  <div class="shell">
    {#if sidebarOpen}
      <div
        class="sidebar-backdrop"
        onclick={() => (sidebarOpen = false)}
        role="presentation"
        tabindex="-1"
      ></div>
    {/if}
    <Sidebar bind:open={sidebarOpen} />
    <main>
      <header class="mobile-topbar">
        <button
          class="btn icon mobile-menu-btn"
          onclick={() => {
            sidebarOpen = !sidebarOpen;
            if (sidebarOpen) userMenuOpen = false;
          }}
          aria-label={sidebarOpen ? __("Close menu") : __("Open menu")}
          aria-expanded={sidebarOpen}
          type="button"
        >
          <Icon name={sidebarOpen ? "x" : "menu"} size={18} />
        </button>

        <a href="/app" class="mobile-brand">
          <span class="logo">{siteLogo()}</span>
          <span class="mobile-brand-title">{siteName()}</span>
          {#if activeWorkspaceName}
            <span class="mobile-ws-badge">{activeWorkspaceName}</span>
          {/if}
        </a>

        <div class="mobile-actions">
          <button class="btn icon mobile-action-btn" onclick={openSearch} aria-label={__("Search")} type="button">
            <Icon name="search" size={18} />
          </button>
          <a href="/app/notifications" class="btn icon mobile-action-btn" aria-label={__("Notifications")}>
            <Icon name="bell" size={18} />
            {#if notifications.unread > 0}
              <span class="notification-count" aria-label={__("{0} unread notifications", [notifications.unread])}>{notifications.unread}</span>
            {/if}
          </a>
          <div class="dropdown mobile-user-dropdown">
            <button
              class="mobile-avatar-btn"
              onclick={() => (userMenuOpen = !userMenuOpen)}
              aria-label={__("User menu")}
              aria-haspopup="menu"
              aria-expanded={userMenuOpen}
              type="button"
            >
              {avatarInitial(displayName)}
            </button>
            {#if userMenuOpen}
              <div class="menu" role="menu">
                <div class="mobile-user-info">
                  <div class="mobile-user-name">{displayName}</div>
                  {#if boot.data?.user && boot.data.user !== displayName}
                    <div class="mobile-user-email">{boot.data.user}</div>
                  {/if}
                </div>
                <div class="mobile-menu-divider"></div>
                <button
                  role="menuitem"
                  onclick={() => { userMenuOpen = false; goto("/app/profile"); }}
                >
                  <Icon name="user" size={14} /> <span>{__("My profile")}</span>
                </button>
                {#if boot.data?.portals?.length}
                  <button role="menuitem" onclick={() => { userMenuOpen = false; goto("/portal"); }}>
                    <Icon name="external-link" size={14} /> <span>{__("My portal")}</span>
                  </button>
                {/if}
                <button
                  role="menuitem"
                  onclick={() => { userMenuOpen = false; openShortcutsHelp(); }}
                >
                  <Icon name="keyboard" size={14} /> <span>{__("Keyboard shortcuts")}</span>
                </button>
                <div class="mobile-menu-divider"></div>
                <button
                  role="menuitem"
                  onclick={handleLogout}
                >
                  <Icon name="log-out" size={14} /> <span>{__("Sign out")}</span>
                </button>
              </div>
            {/if}
          </div>
        </div>
      </header>
      {#if maintenance.enabled}
        <div class="maintenance-banner" role="status">
          <Icon name="alert-triangle" size={14} />
          <span><strong>{__("Maintenance mode")}</strong> — {__("changes are paused until the site reopens.")}{#if maintenance.reason} {maintenance.reason}{/if}</span>
        </div>
      {/if}
      {@render children()}
    </main>
  </div>
{/if}
<Toasts />
<Dialogs />
<ShortcutsModal />
{#if ready && !isLogin && !isPortal && isLoggedIn()}<SearchPalette />{/if}

<style>
  .shell { display: flex; min-height: 100vh; }
  main { flex: 1; min-width: 0; }
  .maintenance-banner {
    display: flex; align-items: center; gap: 8px; padding: 8px 16px;
    background: #fef3c7; color: #92400e; border-bottom: 1px solid #fcd34d; font-size: 13px;
  }
  .mobile-topbar { display: none; }
  .sidebar-backdrop { display: none; }

  @media (max-width: 800px) {
    .mobile-topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      position: sticky;
      top: 0;
      left: 0;
      right: 0;
      height: 52px;
      padding: 0 12px;
      background: #fff;
      border-bottom: 1px solid var(--border);
      z-index: 45;
    }
    .mobile-menu-btn {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      padding: 0;
      border: 0;
      background: none;
      color: inherit;
      cursor: pointer;
    }
    .mobile-brand {
      display: flex;
      align-items: center;
      gap: 8px;
      color: inherit;
      text-decoration: none;
      font-size: 14px;
      font-weight: 600;
      min-width: 0;
    }
    .mobile-brand:hover { text-decoration: none; }
    .mobile-brand .logo {
      display: inline-flex;
      width: 24px;
      height: 24px;
      border-radius: 6px;
      background: var(--primary);
      color: #fff;
      align-items: center;
      justify-content: center;
      font-size: 12px;
      font-weight: 700;
      flex-shrink: 0;
    }
    .mobile-brand-title {
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mobile-ws-badge {
      font-size: 11px;
      font-weight: 500;
      background: #f3f4f6;
      color: var(--muted);
      padding: 1px 6px;
      border-radius: 4px;
      max-width: 110px;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mobile-actions {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .mobile-action-btn {
      position: relative;
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 36px;
      height: 36px;
      padding: 0;
      border: 0;
      background: none;
      color: inherit;
      text-decoration: none;
    }
    .mobile-action-btn .notification-count {
      position: absolute;
      top: 4px;
      right: 4px;
      border-radius: 10px;
      padding: 0 4px;
      background: var(--primary);
      color: #fff;
      font-size: 10px;
      font-weight: 600;
      line-height: 14px;
    }
    .mobile-avatar-btn {
      display: inline-flex;
      width: 28px;
      height: 28px;
      border-radius: 50%;
      background: var(--primary);
      color: #fff;
      align-items: center;
      justify-content: center;
      font-size: 12px;
      font-weight: 600;
      border: 0;
      padding: 0;
      cursor: pointer;
    }
    .mobile-user-dropdown {
      position: relative;
    }
    .mobile-user-dropdown .menu {
      position: absolute;
      right: 0;
      top: calc(100% + 8px);
      min-width: 200px;
      background: #fff;
      border: 1px solid var(--border);
      border-radius: 8px;
      box-shadow: 0 10px 30px rgba(0, 0, 0, 0.12);
      z-index: 60;
      padding: 4px;
    }
    .mobile-user-dropdown .menu button {
      display: flex;
      align-items: center;
      gap: 8px;
      width: 100%;
      text-align: left;
      padding: 8px 10px;
      border: 0;
      background: none;
      border-radius: 5px;
      cursor: pointer;
      font-size: 13px;
      color: var(--text);
    }
    .mobile-user-dropdown .menu button:hover {
      background: #f3f4f6;
    }
    .mobile-user-info {
      padding: 8px 10px 6px;
    }
    .mobile-user-name {
      font-weight: 600;
      font-size: 13px;
      color: var(--text);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .mobile-user-email {
      font-size: 11px;
      color: var(--muted);
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
      margin-top: 2px;
    }
    .mobile-menu-divider {
      height: 1px;
      background: var(--border);
      margin: 4px 0;
    }
    .sidebar-backdrop {
      display: block;
      position: fixed;
      inset: 0;
      background: rgba(0, 0, 0, 0.45);
      z-index: 48;
      backdrop-filter: blur(1px);
      -webkit-backdrop-filter: blur(1px);
      cursor: pointer;
    }
    :global(.page) {
      padding: 16px 16px 24px;
    }
  }
</style>
