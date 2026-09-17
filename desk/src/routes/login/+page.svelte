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
  const providers = $derived(offer?.providers ?? []);
  // with password sign-in off the form stays reachable for Administrator,
  // who keeps a password for the day the identity provider is down
  let showPassword = $state(false);
  const passwordForm = $derived(offer?.password !== false || showPassword);
  const redirect = $derived(page.url.searchParams.get("redirect") || "/app");
  function ssoHref(id: string) {
    return `/api/auth/oidc/${encodeURIComponent(id)}/start?redirect=${encodeURIComponent(redirect)}`;
  }
  // literal keys, one per reason, so the catalogue extractor finds them all
  function ssoMessage(code: string | null): string {
    switch (code) {
      case null: return "";
      case "no_account": return __("There is no account for this e-mail address. Ask an administrator to invite you.");
      case "unverified_email": return __("The provider did not confirm this e-mail address.");
      case "disabled": return __("User is disabled");
      case "domain": return __("This e-mail domain is not allowed to sign in here.");
      case "throttled": return __("Too many attempts. Try again later.");
      case "state": return __("The sign-in expired or was started in another browser. Try again.");
      default: return __("Single sign-on failed. Try again.");
    }
  }
  const ssoError = ssoMessage(page.url.searchParams.get("sso_error"));
  // `autofocus` fails Svelte's a11y check; focus the first field from the effect instead.
  $effect(() => { usrInput?.focus(); });
  async function login(e: Event) {
    e.preventDefault();
    busy = true; error = "";
    try {
      await api.login(usr, pwd);
      location.href = redirect;
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
    {#if ssoError}<div class="err">{ssoError}</div>{/if}
    {#each providers as p (p.id)}
      <a class="btn" href={ssoHref(p.id)} style="width:100%;justify-content:center">{__("Continue with {0}", [p.label])}</a>
    {/each}
    {#if providers.length && passwordForm}<div class="sep small">{__("or")}</div>{/if}
    {#if passwordForm}
      <label>{__("Username")}<input class="input" bind:this={usrInput} bind:value={usr} autocomplete="username" /></label>
      <label>{__("Password")}<input class="input" type="password" bind:value={pwd} autocomplete="current-password" /></label>
      {#if error}<div class="err">{error}</div>{/if}
      <button class="btn primary" bind:this={submit} disabled={busy} style="width:100%;justify-content:center">{__("Sign in")}</button>
      {#if offer?.password !== false}
        <a class="small" href="/login/forgot" style="text-align:center">{__("I forgot my password")}</a>
      {/if}
    {:else}
      <button type="button" class="small link" onclick={() => (showPassword = true)}>{__("Administrator sign-in")}</button>
    {/if}
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
  .sep { text-align: center; color: var(--muted); }
  .link { background: none; border: 0; color: var(--muted); cursor: pointer; text-align: center; }
</style>
