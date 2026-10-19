<script lang="ts">
  // The shell of every dialog: backdrop, focus trap, Escape and backdrop close, head; children are a `.body` and an optional `.foot`.
  import type { Snippet } from "svelte";
  import { focusTrap } from "$lib/focus-trap";
  import { __ } from "$lib/boot.svelte";
  import Icon from "./Icon.svelte";

  let {
    title,
    icon,
    size = "md",
    closable = true,
    class: backdropClass = "",
    onpaste,
    onclose,
    children,
  }: {
    title: string;
    icon?: string;
    size?: "sm" | "md" | "lg";
    /** false while the dialog is busy: the close button is disabled and Escape and the backdrop do nothing. */
    closable?: boolean;
    class?: string;
    onpaste?: (e: ClipboardEvent) => void;
    onclose: () => void;
    children: Snippet;
  } = $props();

  const titleId = $props.id();

  function dismiss() {
    if (closable) onclose();
  }
</script>

<div
  class="modal-bg {backdropClass}"
  use:focusTrap
  role="dialog"
  aria-modal="true"
  aria-labelledby={titleId}
  tabindex="-1"
  {onpaste}
  onclick={(e) => e.target === e.currentTarget && dismiss()}
  onkeydown={(e) => {
    if (e.key !== "Escape" || e.defaultPrevented) return;
    e.stopPropagation();
    dismiss();
  }}
>
  <div class="modal {size}">
    <div class="head">
      <h3 id={titleId}>
        {#if icon}<Icon name={icon} size={18} />{/if}
        {title}
      </h3>
      <button type="button" class="btn icon" onclick={onclose} disabled={!closable} aria-label={__("Close")}>
        <Icon name="x" size={16} />
      </button>
    </div>
    {@render children()}
  </div>
</div>
