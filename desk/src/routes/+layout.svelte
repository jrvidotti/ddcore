<script lang="ts">
  import "../app.css";
  import { __, loadBoot, isLoggedIn, siteName, siteLogo, boot } from "$lib/boot.svelte";
  import { installDeskSDK, loadAppIncludes } from "$lib/desk-sdk";
  import { connectEvents, disconnectEvents } from "$lib/events";
  import { startNotifications, stopNotifications, notifications } from "$lib/notifications.svelte";
  import { startPendingTasks, stopPendingTasks } from "$lib/assignments.svelte";
  import { clearMetaCache } from "$lib/meta";
  import Sidebar from "$lib/components/Sidebar.svelte";
  import Toasts from "$lib/components/Toasts.svelte";
  import Dialogs from "$lib/components/Dialogs.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import ShortcutsModal from "$lib/components/ShortcutsModal.svelte";
  import { ui, toast } from "$lib/ui.svelte";
  import { shouldToggleShortcuts, toggleShortcutsHelp, shortcutsState, closeShortcutsHelp } from "$lib/shortcuts.svelte";
  import { onMessage } from "$lib/api";
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
  const isLogin = $derived(page.url.pathname.startsWith("/login"));

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
    if (shortcutsState.open && e.key === "Escape") {
      closeShortcutsHelp();
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

  // Automatically close sidebar on mobile when navigating to another route
  afterNavigate(() => {
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

  onMount(async () => {
    installDeskSDK();
    onMessage((m) => toast(m.message, { title: m.title, indicator: m.indicator || "blue" }));
    try {
      const b = await loadBoot();
      // app.html ships lang="en"; the boot is what knows the real one
      document.documentElement.lang = b.lang;
      // Awaited: rendering the page before the URL changes lets its own redirect (/app → a
      // workspace) replace this one, and a guest never reaches the sign-in form.
      if (b.user === "Guest" && !isLogin) { await goto("/login?redirect=" + encodeURIComponent(page.url.pathname + page.url.search)); }
      else if (b.user !== "Guest") {
        await loadAppIncludes(b.apps, b.loaded);
        startNotifications();
        startPendingTasks();
        connectEvents(async () => { clearMetaCache(); const nb = await loadBoot(); await loadAppIncludes(nb.apps, nb.loaded); toast(__("Apps reloaded"), { indicator: "blue", timeout: 2000 }); });
      }
    } catch (e) { console.error(e); }
    ready = true;
  });
</script>

<svelte:head><title>{siteName()}</title></svelte:head>
<svelte:window onkeydown={onWindowKeydown} />

{#if ui.busy > 0}<div class="busy-bar"></div>{/if}
{#if !ready}
  <div class="page muted">…</div>
{:else if isLogin || !isLoggedIn()}
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
          onclick={() => (sidebarOpen = !sidebarOpen)}
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
          <a href="/app/notifications" class="btn icon mobile-action-btn" aria-label={__("Notifications")}>
            <Icon name="bell" size={18} />
            {#if notifications.unread > 0}
              <span class="notification-count" aria-label={__("{0} unread notifications", [notifications.unread])}>{notifications.unread}</span>
            {/if}
          </a>
          <a href="/app/profile" class="mobile-avatar" aria-label={__("My profile")}>
            {avatarInitial(displayName)}
          </a>
        </div>
      </header>
      {@render children()}
    </main>
  </div>
{/if}
<Toasts />
<Dialogs />
<ShortcutsModal />

<style>
  .shell { display: flex; min-height: 100vh; }
  main { flex: 1; min-width: 0; }
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
    .mobile-avatar {
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
      text-decoration: none;
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
