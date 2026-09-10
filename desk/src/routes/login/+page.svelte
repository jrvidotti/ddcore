<script lang="ts">
  import { api } from "$lib/api";
  import { page } from "$app/state";
  let usr = $state(""), pwd = $state(""), error = $state(""), busy = $state(false);
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
    <div class="logo">c</div>
    <h1>Entrar</h1>
    <label>Usuário<input class="input" bind:value={usr} autocomplete="username" autofocus /></label>
    <label>Senha<input class="input" type="password" bind:value={pwd} autocomplete="current-password" /></label>
    {#if error}<div class="err">{error}</div>{/if}
    <button class="btn primary" disabled={busy} style="width:100%;justify-content:center">Entrar</button>
  </form>
</div>

<style>
  .wrap { min-height: 100vh; display: flex; align-items: center; justify-content: center; padding: 16px; }
  .box { width: 100%; max-width: 360px; padding: 28px; display: flex; flex-direction: column; gap: 12px; }
  .logo { width: 40px; height: 40px; border-radius: 10px; background: var(--primary); color: #fff; display: flex; align-items: center; justify-content: center; font-weight: 700; font-size: 20px; }
  h1 { font-size: 18px; }
  label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .err { color: var(--red); font-size: 13px; }
</style>
