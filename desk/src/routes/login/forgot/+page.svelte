<script lang="ts">
  import SiteMark from "$lib/components/SiteMark.svelte";
  import { api } from "$lib/api";
  import { __, siteName } from "$lib/boot.svelte";

  let usr = $state(""), busy = $state(false), sent = $state(false), error = $state("");

  async function submit(e: Event) {
    e.preventDefault();
    busy = true; error = "";
    try {
      await api.forgotPassword(usr);
      // The server answers the same way for an address it knows, one it does
      // not, and a disabled account — so this screen says the same thing too.
      // Saying "we sent it" only when the address exists would give away, from
      // the outside, exactly what the endpoint refuses to give away.
      sent = true;
    } catch (err: any) {
      // The one thing that does come back differently is a 429.
      error = err.message;
    } finally { busy = false; }
  }
</script>

<div class="wrap">
  {#if sent}
    <div class="card box">
      <div class="logo"><SiteMark size={40} alt={siteName()} /></div>
      <h1>{__("Check your email")}</h1>
      <p class="small muted">
        {__("If that address belongs to an account, a link to choose a new password is on its way. The link can only be used once, and expires.")}
      </p>
      <a class="btn" href="/login" style="justify-content:center">{__("Back to sign in")}</a>
    </div>
  {:else}
    <form class="card box" onsubmit={submit}>
      <div class="logo"><SiteMark size={40} alt={siteName()} /></div>
      <h1>{__("Forgotten password")}</h1>
      <p class="small muted">{__("Tell us the address you sign in with and we will send a link to set a new password.")}</p>
      <label>{__("Email")}<input class="input" bind:value={usr} autocomplete="username" /></label>
      {#if error}<div class="err">{error}</div>{/if}
      <button class="btn primary" disabled={busy} style="width:100%;justify-content:center">{__("Send the link")}</button>
      <a class="small" href="/login" style="text-align:center">{__("Back to sign in")}</a>
    </form>
  {/if}
</div>

<style>
  .wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px; }
  .box { width: 100%; max-width: 360px; padding: 28px; display: flex; flex-direction: column; gap: 12px; }
  .logo { display: flex; }
  h1 { font-size: 18px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .err { color: var(--red); font-size: 13px; }
  p { margin: 0; }
</style>
