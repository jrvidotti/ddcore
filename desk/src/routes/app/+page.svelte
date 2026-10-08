<script lang="ts">
  import { seg } from "$lib/routes";
  import { boot, __, spaceLabel } from "$lib/boot.svelte";
  import { goto } from "$app/navigation";
  import Spinner from "$lib/components/Spinner.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { getRememberedWorkspace, landingWorkspace } from "$lib/components/sidebar-workspace";
  import { accountLabel, emptyHome } from "$lib/home";
  import { signOut } from "$lib/sign-out";

  $effect(() => {
    if (boot.ready) {
      const target = landingWorkspace(boot.data?.workspaces || [], getRememberedWorkspace(), boot.data?.site?.home);
      if (target) {
        goto(`/app/${seg(target)}`, { replaceState: true });
      }
    }
  });

  const empty = $derived(boot.ready ? emptyHome(boot.data) : null);
  const account = $derived(accountLabel(boot.data?.user || "", boot.data?.userDoc?.full_name));
  const space = $derived(spaceLabel());
</script>

{#if empty === "no-access"}
  <div class="page">
    <div class="card no-access">
      <Icon name="lock" size={28} />
      <h1>{__("You do not have access to any module yet")}</h1>
      <p>
        {space
          ? __("You are signed in as {0} in {1}, but none of your roles opens a module.", [account, space])
          : __("You are signed in as {0}, but none of your roles opens a module.", [account])}
      </p>
      <p>{__("Ask an administrator to give you a role, then sign in again.")}</p>
      <button class="btn" onclick={signOut}><Icon name="log-out" size={14} /> {__("Sign out")}</button>
    </div>
  </div>
{:else if empty === "none-declared"}
  <div class="page"><div class="card empty">{__("No workspace declared. Create an app with ddcore new-app and declare a workspace.")}</div></div>
{:else}
  <div class="page"><Spinner /></div>
{/if}

<style>
  .no-access { max-width: 520px; margin: 40px auto; padding: 32px; text-align: center; color: var(--muted); display: flex; flex-direction: column; align-items: center; gap: 8px; }
  .no-access h1 { font-size: 18px; color: var(--text); margin: 4px 0; }
  .no-access p { margin: 0; line-height: 1.5; }
  .no-access .btn { margin-top: 12px; }
</style>
