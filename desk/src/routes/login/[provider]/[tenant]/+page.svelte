<script lang="ts">
  // One organization's sign-in through an app's credential provider (#117):
  // /login/<provider>/<tenant>. The organization was typed, never listed, and
  // this page shows its name before asking for the username and password.
  import { api } from "$lib/api";
  import { __, boot, siteLogo } from "$lib/boot.svelte";
  import { page } from "$app/state";
  import { landing } from "$lib/portal";
  import { METHOD_KEY, normalizeTenant, remember, tenantKey } from "$lib/login-methods";
  const providerId = $derived(page.params.provider ?? "");
  const tenantId = $derived(normalizeTenant(page.params.tenant ?? ""));
  const provider = $derived(boot.data?.site?.login?.credentials?.find((c) => c.id === providerId));
  let org: { id: string; title: string } | null = $state(null);
  let phase: "loading" | "ready" | "missing" | "error" = $state("loading");
  let usr = $state(""), pwd = $state(""), error = $state(""), busy = $state(false);
  let usrInput: HTMLInputElement | undefined = $state();
  const asked = $derived(page.url.searchParams.get("redirect"));
  $effect(() => {
    const p = providerId, t = tenantId;
    phase = "loading"; error = "";
    api.credentialTenant(p, t).then((r) => {
      if (p !== providerId || t !== tenantId) return;
      org = r?.data ?? null;
      phase = org ? "ready" : "missing";
    }).catch((err: any) => {
      if (p !== providerId || t !== tenantId) return;
      phase = err?.status === 404 ? "missing" : "error";
      error = err?.status === 404 ? "" : err.message;
    });
  });
  $effect(() => { if (phase === "ready") usrInput?.focus(); });
  // back to /login, on this provider's tab
  function back() { remember(METHOD_KEY, "cred:" + providerId); }
  async function login(e: Event) {
    e.preventDefault();
    if (!org) return;
    busy = true; error = "";
    try {
      const r = await api.credentialLogin(providerId, org.id, usr, pwd);
      remember(tenantKey(providerId), org.id);
      location.href = landing(r?.home, asked);
    } catch (err: any) { error = err.message; } finally { busy = false; }
  }
</script>

<div class="wrap">
  <form class="card box" onsubmit={login}>
    <div class="logo">{siteLogo()}</div>
    {#if phase === "loading"}
      <p class="muted">{__("Loading…")}</p>
    {:else if phase === "ready" && org}
      <div class="org">
        <h1>{org.title}</h1>
        {#if provider}<div class="small muted">{__("Sign in with {0}", [provider.label])}</div>{/if}
      </div>
      <label>{__("Username")}<input class="input" bind:this={usrInput} bind:value={usr} autocomplete="username" autocapitalize="none" /></label>
      <label>{__("Password")}<input class="input" type="password" bind:value={pwd} autocomplete="current-password" /></label>
      {#if error}<div class="err">{error}</div>{/if}
      <button class="btn primary" disabled={busy} style="width:100%;justify-content:center">{__("Sign in")}</button>
      <a class="small back" href="/login" onclick={back}>{__("Not your organization?")}</a>
    {:else}
      <h1>{__("Sign in")}</h1>
      <div class="err">{phase === "missing" ? __("There is no organization {0} here", [tenantId]) : error}</div>
      <a class="small back" href="/login" onclick={back}>{__("Back")}</a>
    {/if}
  </form>
</div>

<style>
  .wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px; }
  .box { width: 100%; max-width: 360px; padding: 28px; display: flex; flex-direction: column; gap: 12px; }
  .logo { width: 40px; height: 40px; border-radius: 10px; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 20px; overflow: hidden; }
  h1 { font-size: 18px; overflow-wrap: anywhere; }
  .org { display: flex; flex-direction: column; gap: 2px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .err { color: var(--red); font-size: 13px; }
  .muted { color: var(--muted); }
  .back { text-align: center; }
</style>
