<script lang="ts">
  import { api } from "$lib/api";
  import { __, boot, siteLogo } from "$lib/boot.svelte";
  import { page } from "$app/state";
  let usr = $state(""), pwd = $state(""), error = $state(""), busy = $state(false);
  let usrInput: HTMLInputElement | undefined = $state();
  let submit: HTMLButtonElement | undefined = $state();
  // the deployment's notice and demo account, if it declares them
  const offer = $derived(boot.data?.site?.login);
  function useDemo() {
    usr = offer?.demoUser || "";
    pwd = offer?.demoPassword || "";
    submit?.focus();
  }
  // `autofocus` fails Svelte's a11y check; focus the first field from the effect instead.
  $effect(() => { usrInput?.focus(); });
  async function login(e: Event) {
    e.preventDefault();
    busy = true; error = "";
    try {
      await api.login(usr, pwd);
      location.href = page.url.searchParams.get("redirect") || "/app";
    } catch (err: any) { error = err.message; } finally { busy = false; }
  }
</script>

<div class="wrap">
  <form class="card box" onsubmit={login}>
    <div class="logo">{siteLogo()}</div>
    <h1>{__("Sign in")}</h1>
    {#if offer?.notice || offer?.demoUser}
      <div class="notice">
        {#if offer.notice}<p>{offer.notice}</p>{/if}
        {#if offer.demoUser}
          <dl>
            <dt>{__("Username")}</dt><dd><code>{offer.demoUser}</code></dd>
            <dt>{__("Password")}</dt><dd><code>{offer.demoPassword}</code></dd>
          </dl>
          <button type="button" class="btn" onclick={useDemo}>{__("Use demo account")}</button>
        {/if}
      </div>
    {/if}
    <label>{__("Username")}<input class="input" bind:this={usrInput} bind:value={usr} autocomplete="username" /></label>
    <label>{__("Password")}<input class="input" type="password" bind:value={pwd} autocomplete="current-password" /></label>
    {#if error}<div class="err">{error}</div>{/if}
    <button class="btn primary" bind:this={submit} disabled={busy} style="width:100%;justify-content:center">{__("Sign in")}</button>
    <a class="small" href="/login/forgot" style="text-align:center">{__("I forgot my password")}</a>
  </form>
</div>

<style>
  .wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px; }
  .box { width: 100%; max-width: 360px; padding: 28px; display: flex; flex-direction: column; gap: 12px; }
  .logo { width: 40px; height: 40px; border-radius: 10px; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 20px; overflow: hidden; }
  h1 { font-size: 18px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .err { color: var(--red); font-size: 13px; }
  .notice { background: var(--bg); border: 1px solid var(--border); border-radius: var(--radius); padding: 10px 12px; font-size: 13px; display: flex; flex-direction: column; gap: 8px; }
  .notice p { margin: 0; white-space: pre-line; }
  .notice dl { margin: 0; display: grid; grid-template-columns: auto 1fr; gap: 2px 10px; align-items: baseline; }
  .notice dt { font-size: 12px; color: var(--muted); }
  .notice dd { margin: 0; overflow-wrap: anywhere; }
  .notice .btn { align-self: flex-start; }
</style>
