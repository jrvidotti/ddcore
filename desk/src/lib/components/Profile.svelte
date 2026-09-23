<script lang="ts">
  import { boot, __, loadBoot } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { toast, showError, confirm } from "$lib/ui.svelte";
  import { resetLocale } from "$lib/locale";
  import { formatDatetime } from "$lib/format";
  import Icon from "./Icon.svelte";
  import Control from "$lib/controls/Control.svelte";
  import { describeDevice, isExpired, languageField, passwordProblem, sortSessions, type SessionRow } from "./profile";
  import { onMount } from "svelte";

  type Profile = {
    id: string; email: string; fullName: string;
    language: string | null; userType: string; lastLogin: string | null; roles: string[];
  };

  let profile = $state<Profile | null>(null);
  let sessions = $state<SessionRow[]>([]);
  let keys = $state<any[]>([]);
  const langField = $derived(languageField(boot.data?.langs || [], __));

  let fullName = $state("");
  let language = $state<string | null>(null);
  let savingProfile = $state(false);

  let current = $state(""), pwd = $state(""), confirmPwd = $state("");
  let pwdError = $state(""), savingPwd = $state(false);

  let newKeyLabel = $state(""), newKeyDays = $state("");
  let mintedToken = $state("");

  const minLength = 8;

  onMount(load);

  async function load() {
    try {
      profile = await api.call("core.services.profile.getMyProfile");
      fullName = profile!.fullName;
      language = profile!.language;
      await Promise.all([loadSessions(), loadKeys()]);
    } catch (e) { showError(e); }
  }

  async function loadSessions() {
    const r = await api.call("core.services.sessions.listMySessions");
    sessions = sortSessions(r.sessions || []);
  }

  async function loadKeys() {
    const r = await api.call("core.services.api_keys.listMyAPIKeys");
    keys = r.keys || [];
  }

  async function saveProfile() {
    savingProfile = true;
    try {
      const before = profile?.language ?? null;
      await api.call("core.services.profile.updateMyProfile", { fullName, language });
      if ((language ?? null) !== before) {
        // The language is resolved server-side per request, so the catalogue
        // has to be fetched again before anything re-renders in it.
        await loadBoot();
        resetLocale();
        document.documentElement.lang = boot.data?.lang || "en";
      }
      profile = await api.call("core.services.profile.getMyProfile");
      toast(__("Profile saved"), { indicator: "green" });
    } catch (e) { showError(e); } finally { savingProfile = false; }
  }

  async function changePassword(e: Event) {
    e.preventDefault();
    pwdError = passwordProblem(pwd, confirmPwd, minLength, __);
    if (pwdError) return;
    savingPwd = true;
    try {
      await api.call("core.services.profile.changeMyPassword", { current, password: pwd });
      current = pwd = confirmPwd = "";
      await loadSessions();
      toast(__("Password changed. Your other sessions were signed out."), { indicator: "green" });
    } catch (err: any) { pwdError = err.message; } finally { savingPwd = false; }
  }

  async function revokeSession(row: SessionRow) {
    if (!(await confirm(__("End this session?")))) return;
    try {
      await api.call("core.services.sessions.revokeMySession", { id: row.id });
      await loadSessions();
      toast(__("Session ended"), { indicator: "green" });
    } catch (e) { showError(e); }
  }

  async function revokeOthers() {
    if (!(await confirm(__("Sign out of every other device?")))) return;
    try {
      const r = await api.call("core.services.sessions.revokeMyOtherSessions");
      await loadSessions();
      toast(__("{0} session(s) ended", [r.revoked]), { indicator: "green" });
    } catch (e) { showError(e); }
  }

  async function createKey(e: Event) {
    e.preventDefault();
    try {
      const r = await api.call("core.services.api_keys.createMyAPIKey", {
        label: newKeyLabel, days: Number(newKeyDays || 0),
      });
      mintedToken = r.token;
      newKeyLabel = ""; newKeyDays = "";
      await loadKeys();
    } catch (e) { showError(e); }
  }

  async function revokeKey(k: any) {
    if (!(await confirm(__("Revoke the key {0}?", [k.label || k.id]), undefined, { destructive: true }))) return;
    try {
      await api.call("core.services.api_keys.revokeMyAPIKey", { id: k.id });
      await loadKeys();
      toast(__("Key revoked"), { indicator: "green" });
    } catch (e) { showError(e); }
  }

  const when = (v: any) => (v ? formatDatetime(v) : "—");
</script>

<div class="page">
  <div class="page-head"><h1>{__("My profile")}</h1></div>

  {#if !profile}
    <div class="card" style="padding:16px 20px">{__("Loading…")}</div>
  {:else}
    <div class="card sect">
      <h2>{__("Account")}</h2>
      <div class="grid">
        <div><span class="lbl">{__("Email")}</span><span>{profile.email}</span></div>
        <div><span class="lbl">{__("Type")}</span><span>{profile.userType || "—"}</span></div>
        <div><span class="lbl">{__("Last login")}</span><span>{when(profile.lastLogin)}</span></div>
        <div>
          <span class="lbl">{__("Roles")}</span>
          <span>
            {#if profile.roles.length}
              {#each profile.roles as r}<span class="chip">{r}</span>{/each}
            {:else}—{/if}
          </span>
        </div>
      </div>
      <p class="small muted" style="margin:10px 0 0">
        {__("The email address names the account and cannot be changed here. Roles are granted by an admin.")}
      </p>
    </div>

    <div class="card sect">
      <h2>{__("Name and language")}</h2>
      <div class="row top">
        <label class="fld">
          <span class="lbl">{__("Full name")}</span>
          <input class="input" bind:value={fullName} />
        </label>
        {#if langField.options.length}
          <div class="fld">
            <Control field={langField} value={language} compact onchange={(v: any) => (language = v || null)} />
          </div>
        {/if}
      </div>
      <button class="btn primary" disabled={savingProfile} onclick={saveProfile}>{__("Save")}</button>
    </div>

    <form class="card sect" onsubmit={changePassword}>
      <h2>{__("Password")}</h2>
      <div class="row">
        <label class="fld">
          <span class="lbl">{__("Current password")}</span>
          <input class="input" type="password" autocomplete="current-password" bind:value={current} />
        </label>
        <label class="fld">
          <span class="lbl">{__("New password")}</span>
          <input class="input" type="password" autocomplete="new-password" bind:value={pwd} />
        </label>
        <label class="fld">
          <span class="lbl">{__("Confirm the new password")}</span>
          <input class="input" type="password" autocomplete="new-password" bind:value={confirmPwd} />
        </label>
      </div>
      {#if pwdError}<div class="err small">{pwdError}</div>{/if}
      <p class="small muted">{__("Changing the password signs out every other device.")}</p>
      <button class="btn primary" disabled={savingPwd}>{__("Change password")}</button>
    </form>

    <div class="card sect">
      <div class="head">
        <h2>{__("Active sessions")}</h2>
        <button class="btn sm" onclick={revokeOthers}>{__("Sign out everywhere else")}</button>
      </div>
      <table class="grid">
        <thead><tr>
          <th>{__("Device")}</th><th>{__("Address")}</th><th>{__("Last seen")}</th><th></th>
        </tr></thead>
        <tbody>
          {#each sessions as s}
            <tr>
              <td>
                {describeDevice(s.userAgent, __("Unknown device"))}
                {#if s.current}<span class="chip green">{__("This device")}</span>{/if}
              </td>
              <td class="small muted">{s.ip || "—"}</td>
              <td class="small muted">{when(s.lastSeen)}</td>
              <td style="text-align:right">
                {#if !s.current}
                  <button class="btn sm danger" onclick={() => revokeSession(s)}>{__("End")}</button>
                {/if}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>

    <div class="card sect">
      <h2>{__("API keys")}</h2>
      {#if mintedToken}
        <div class="minted">
          <strong>{__("Copy this key now — it is not shown again.")}</strong>
          <code>{mintedToken}</code>
          <button class="btn sm" onclick={() => (mintedToken = "")}>{__("Done")}</button>
        </div>
      {/if}
      <form class="row" onsubmit={createKey}>
        <label class="fld">
          <span class="lbl">{__("What is it for?")}</span>
          <input class="input" bind:value={newKeyLabel} placeholder={__("e.g. the invoicing script")} />
        </label>
        <label class="fld" style="max-width:160px">
          <span class="lbl">{__("Expires in (days)")}</span>
          <input class="input" type="number" min="0" bind:value={newKeyDays} placeholder={__("never")} />
        </label>
        <button class="btn">{__("Create key")}</button>
      </form>
      {#if keys.length}
        <table class="grid">
          <thead><tr>
            <th>{__("Description")}</th><th>{__("Last used")}</th><th>{__("Expires")}</th><th></th>
          </tr></thead>
          <tbody>
            {#each keys as k}
              <tr class:expired={isExpired(k.expires)}>
                <td>{k.label || k.id}</td>
                <td class="small muted">{when(k.last_used)}</td>
                <td class="small muted">{k.expires ? when(k.expires) : __("never")}</td>
                <td style="text-align:right">
                  <button class="btn sm danger" onclick={() => revokeKey(k)}>{__("Revoke")}</button>
                </td>
              </tr>
            {/each}
          </tbody>
        </table>
      {:else}
        <p class="small muted">{__("No keys yet.")}</p>
      {/if}
    </div>
  {/if}
</div>

<style>
  .sect { padding: 16px 20px; margin-bottom: 16px; }
  .sect h2 { font-size: 14px; margin: 0 0 12px; }
  .head { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
  .grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 12px 20px; }
  .grid > div { display: flex; flex-direction: column; gap: 2px; }
  .lbl { font-size: 11px; text-transform: uppercase; letter-spacing: .05em; color: var(--muted); }
  .row { display: flex; flex-wrap: wrap; gap: 12px; align-items: flex-end; margin-bottom: 12px; }
  /* a row holding a Control: its label, input and description make it taller
     than a bare input, so the fields line up by their tops, not their bottoms */
  .row.top { align-items: flex-start; }
  /* a label over an input is the caption of a control, not a data label: it
     matches what Control renders, so a hand-written field and a Control can
     sit side by side in the same row */
  .fld > .lbl { font-size: 12px; text-transform: none; letter-spacing: normal; }
  .fld { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 200px; }
  .chip { display: inline-block; padding: 1px 7px; border-radius: 999px; background: #f3f4f6; font-size: 11px; margin-right: 4px; }
  .chip.green { background: #dcfce7; color: #166534; }
  .err { color: var(--red); margin-bottom: 8px; }
  .minted { display: flex; flex-direction: column; gap: 6px; padding: 10px 12px; margin-bottom: 12px; border: 1px solid var(--border); border-radius: 6px; background: #fffbeb; }
  .minted code { word-break: break-all; font-size: 12px; }
  table.grid { display: table; width: 100%; }
  tr.expired td { opacity: .55; }
</style>
