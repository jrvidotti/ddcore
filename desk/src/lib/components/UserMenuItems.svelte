<script lang="ts">
  // The items of the user menu, in the sidebar and in the mobile header; onpick closes the menu around them.
  import { goto } from "$app/navigation";
  import { boot, __ } from "$lib/boot.svelte";
  import { openFeedback } from "$lib/feedback.svelte";
  import { openShortcutsHelp } from "$lib/shortcuts.svelte";
  import { signOut } from "$lib/sign-out";
  import { stopImpersonating } from "$lib/impersonation";
  import { showError } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";
  import TenantMenu from "./TenantMenu.svelte";

  let { onpick }: { onpick: () => void } = $props();
  const pick = (action: () => unknown) => () => { onpick(); action(); };
</script>

<TenantMenu {onpick} />
<button role="menuitem" onclick={pick(() => goto("/app/profile"))}>
  <Icon name="user" size={14} /> <span>{__("My profile")}</span>
</button>
{#if boot.data?.portals?.length}
  <!-- a desk user who is also, say, an employee reaches their own portal (OPS-10) -->
  <button role="menuitem" onclick={pick(() => goto("/portal"))}>
    <Icon name="external-link" size={14} /> <span>{__("My portal")}</span>
  </button>
{/if}
<button role="menuitem" onclick={pick(openShortcutsHelp)}>
  <Icon name="keyboard" size={14} /> <span>{__("Keyboard shortcuts")}</span>
</button>
{#if boot.data?.site?.feedback}
  <button role="menuitem" onclick={pick(openFeedback)}>
    <Icon name="message-square" size={14} /> <span>{__("Feedback")}</span>
  </button>
{/if}
{#if boot.data?.impersonatedBy}
  <button role="menuitem" onclick={pick(() => stopImpersonating().catch(showError))}>
    <Icon name="user-x" size={14} /> <span>{__("Stop impersonating")}</span>
  </button>
{/if}
<button role="menuitem" onclick={pick(signOut)}>
  <Icon name="log-out" size={14} /> <span>{__("Sign out")}</span>
</button>
