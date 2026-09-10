<script lang="ts">
  import { boot, __, isLoggedIn } from "$lib/boot.svelte";
  import { openShortcutsHelp } from "$lib/shortcuts.svelte";
  import Icon from "./Icon.svelte";
  import { page } from "$app/state";
  import { api } from "$lib/api";
  import { goto } from "$app/navigation";

  let { open = $bindable(true) }: { open?: boolean } = $props();
  const workspaces = $derived(boot.data?.workspaces || []);
  const current = $derived(page.url.pathname);
  const active = (href: string) => current === href || current.startsWith(href + "/") || current.startsWith(href + "?");
  const itemHref = (it: any) => it.route || (it.doctype ? `/app/${encodeURIComponent(it.doctype)}` : it.report ? `/app/report/${encodeURIComponent(it.report)}` : "");
  async function logout() { await api.logout(); location.href = "/login"; }
  const otherDoctypes = $derived(Object.entries(boot.data?.doctypes || {}).filter(([n, d]) => d.app === "core").sort());
  let showCore = $state(false);
</script>

<aside class="sidebar" class:open>
  <div class="brand">
    <a href="/app" style="display:flex;align-items:center;gap:8px;color:inherit;text-decoration:none"><span class="logo">c</span><strong>{boot.data?.site?.name || "cerne"}</strong></a>
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
    <div class="group" style="cursor:pointer" onclick={() => (showCore = !showCore)} role="button" tabindex="0" onkeydown={(e) => e.key === "Enter" && (showCore = !showCore)}>
      {__("Sistema")} <Icon name={showCore ? "chevron-down" : "chevron-right"} size={12} />
    </div>
    {#if showCore}
      {#each otherDoctypes as [name, d]}
        <a href={`/app/${encodeURIComponent(name)}`} class:active={active(`/app/${encodeURIComponent(name)}`)}><Icon name={d.icon || "circle"} size={d.icon ? 16 : 6} /><span>{d.label}</span></a>
      {/each}
    {/if}
  </nav>
  <div class="foot">
    <div class="small" style="overflow:hidden;text-overflow:ellipsis"><Icon name="user" size={14} /> {boot.data?.userDoc?.full_name || boot.data?.user}</div>
    <div style="display:flex;align-items:center;gap:4px">
      <button class="btn sm icon" onclick={openShortcutsHelp} title="{__('Atalhos de teclado')} (?)"><Icon name="keyboard" size={14} /></button>
      <button class="btn sm icon" onclick={logout} title={__("Sair")}><Icon name="log-out" size={14} /></button>
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
  .foot { padding: 10px 14px; border-top: 1px solid var(--border); display: flex; align-items: center; justify-content: space-between; gap: 8px; }
  @media (max-width: 800px) { .sidebar { position: fixed; z-index: 50; transform: translateX(-100%); transition: transform .15s; } .sidebar.open { transform: none; } }
</style>
