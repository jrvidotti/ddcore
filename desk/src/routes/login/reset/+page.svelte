<script lang="ts">
  import { api } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { page } from "$app/state";
  import { passwordProblem } from "$lib/components/profile";
  import { onMount } from "svelte";

  const token = $derived(page.url.searchParams.get("token") || "");

  let info = $state<{ kind: string; user: string; fullName: string } | null>(null);
  let loadError = $state("");
  let pwd = $state(""), confirm = $state(""), fullName = $state("");
  let error = $state(""), busy = $state(false), done = $state(false);

  const minLength = 8;
  const isInvite = $derived(info?.kind === "invite");

  onMount(async () => {
    if (!token) { loadError = __("This link is no longer valid. Ask for a new one."); return; }
    try {
      // Reading the link does not spend it — this is only so the page can say
      // whose account it is and tell an invitation from a recovery.
      info = await api.authToken(token);
      fullName = info.fullName || "";
    } catch (e: any) { loadError = e.message; }
  });

  async function submit(e: Event) {
    e.preventDefault();
    error = passwordProblem(pwd, confirm, minLength, __);
    if (error) return;
    busy = true;
    try {
      if (isInvite) await api.acceptInvite(token, pwd, fullName);
      else await api.resetPassword(token, pwd);
      done = true;
    } catch (err: any) { error = err.message; } finally { busy = false; }
  }
</script>

<div class="wrap">
  {#if loadError}
    <div class="card box">
      <div class="logo">c</div>
      <h1>{__("This link does not work")}</h1>
      <p class="small muted">{loadError}</p>
      <a class="btn" href="/login/forgot" style="justify-content:center">{__("Ask for a new link")}</a>
    </div>
  {:else if done}
    <div class="card box">
      <div class="logo">c</div>
      <h1>{__("Password set")}</h1>
      <!-- Deliberately not signed in: a session minted from a link would be a
           second way to get one, and one is enough to reason about. -->
      <p class="small muted">{__("Sign in with the password you just chose.")}</p>
      <a class="btn primary" href="/login" style="justify-content:center">{__("Sign in")}</a>
    </div>
  {:else if info}
    <form class="card box" onsubmit={submit}>
      <div class="logo">c</div>
      <h1>{isInvite ? __("Welcome") : __("New password")}</h1>
      <p class="small muted">
        {isInvite
          ? __("Choose a password for {0}.", [info.user])
          : __("Choose a new password for {0}.", [info.user])}
      </p>
      {#if isInvite}
        <label>{__("Full name")}<input class="input" bind:value={fullName} autocomplete="name" /></label>
      {/if}
      <label>{__("New password")}<input class="input" type="password" autocomplete="new-password" bind:value={pwd} /></label>
      <label>{__("Confirm the new password")}<input class="input" type="password" autocomplete="new-password" bind:value={confirm} /></label>
      {#if error}<div class="err">{error}</div>{/if}
      <button class="btn primary" disabled={busy} style="width:100%;justify-content:center">
        {isInvite ? __("Set my password") : __("Change my password")}
      </button>
    </form>
  {:else}
    <div class="card box"><div class="logo">c</div><p class="small muted">{__("Loading…")}</p></div>
  {/if}
</div>

<style>
  .wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px; }
  .box { width: 100%; max-width: 360px; padding: 28px; display: flex; flex-direction: column; gap: 12px; }
  .logo { width: 40px; height: 40px; border-radius: 10px; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 20px; }
  h1 { font-size: 18px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .err { color: var(--red); font-size: 13px; }
  p { margin: 0; }
</style>
