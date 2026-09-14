<script lang="ts">
  import "../app.css";
  import { __, loadBoot, isLoggedIn, siteName } from "$lib/boot.svelte";
  import { installDeskSDK, loadAppIncludes } from "$lib/desk-sdk";
  import { connectEvents, disconnectEvents } from "$lib/events";
  import { startNotifications, stopNotifications } from "$lib/notifications.svelte";
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
  import { goto } from "$app/navigation";
  import { onMount, onDestroy } from "svelte";

  let { children } = $props();
  let ready = $state(false);
  let sidebarOpen = $state(false);
  const isLogin = $derived(page.url.pathname.startsWith("/login"));

  function onWindowKeydown(e: KeyboardEvent) {
    if (shortcutsState.open && e.key === "Escape") {
      closeShortcutsHelp();
      return;
    }
    if (shouldToggleShortcuts(e)) {
      e.preventDefault();
      toggleShortcutsHelp();
    }
  }

  $effect(() => {
    if (isLogin) { stopNotifications(); stopPendingTasks(); disconnectEvents(); }
  });
  onDestroy(() => { stopNotifications(); stopPendingTasks(); disconnectEvents(); });

  onMount(async () => {
    installDeskSDK();
    onMessage((m) => toast(m.message, { title: m.title, indicator: m.indicator || "blue" }));
    try {
      const b = await loadBoot();
      // app.html ships lang="en"; the boot is what knows the real one
      document.documentElement.lang = b.lang;
      if (b.user === "Guest" && !isLogin) { goto("/login?redirect=" + encodeURIComponent(page.url.pathname + page.url.search)); }
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
    <Sidebar bind:open={sidebarOpen} />
    <main>
      <button class="btn icon hamburger" onclick={() => (sidebarOpen = !sidebarOpen)} aria-label="Menu"><Icon name="menu" /></button>
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
  .hamburger { display: none; position: fixed; top: 10px; left: 10px; z-index: 51; }
  @media (max-width: 800px) { .hamburger { display: inline-flex; } :global(.page) { padding: 56px 16px 20px; } }
</style>
