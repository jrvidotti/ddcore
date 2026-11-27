<script lang="ts">
  // A description behind an info icon after a label — a field's or a report filter's
  // `descriptionPlacement: "tooltip"` (#176). It opens while a mouse is over it or the
  // keyboard focus is on it, and a click or a tap keeps it open (a touch screen has no
  // hover); Escape or a press outside closes it. The text stays in the DOM, so the button's
  // aria-describedby gives it to a screen reader whether the tip shows or not.
  import Icon from "$lib/components/Icon.svelte";
  import { __ } from "$lib/boot.svelte";
  import { anchored, dismiss } from "./floating";

  let { text, id, label = "" }: { text: string; id: string; label?: string } = $props();

  let hovered = $state(false);
  let focused = $state(false);
  let pinned = $state(false);
  // a press focuses the button too (in most engines); only a keyboard focus opens the tip
  let pressing = false;
  let btn: HTMLButtonElement | undefined = $state();
  const shown = $derived(hovered || focused || pinned);
  const close = () => { hovered = focused = pinned = false; };
</script>

<span class="info-tip" use:dismiss={shown ? close : null}>
  <button bind:this={btn} type="button" class="info-btn" aria-label={label ? __("About {0}", [label]) : __("More information")}
    aria-describedby={id} aria-expanded={shown}
    onpointerenter={(e) => { if (e.pointerType === "mouse") hovered = true; }}
    onpointerleave={(e) => { if (e.pointerType === "mouse") hovered = false; }}
    onpointerdown={() => (pressing = true)}
    onfocus={() => { if (!pressing) focused = true; pressing = false; }}
    onblur={() => { focused = false; pinned = false; pressing = false; }}
    onclick={() => (pinned = !pinned)}><Icon name="info" size={13} /></button>
  <span {id} class="sr-only">{text}</span>
  {#if shown}
    <span class="tip" aria-hidden="true" use:anchored={{ anchor: btn, content: text }}>{text}</span>
  {/if}
</span>

<style>
  .info-tip { display: inline-flex; align-items: center; vertical-align: middle; }
  .info-btn { all: unset; display: inline-flex; align-items: center; color: var(--muted); cursor: help; border-radius: 50%; line-height: 0; }
  .info-btn:hover, .info-btn[aria-expanded="true"] { color: var(--text); }
  .info-btn:focus-visible { outline: 2px solid var(--primary); outline-offset: 1px; }
  /* fixed from the start, so it is measured at its own width and not the label's */
  .tip {
    position: fixed; z-index: 60; width: max-content; max-width: min(300px, calc(100vw - 16px));
    padding: 8px 10px; border-radius: 6px; background: var(--text); color: var(--surface);
    box-shadow: var(--shadow-pop); font-size: 12px; font-weight: normal; line-height: 1.45;
    white-space: pre-line; text-transform: none; letter-spacing: normal; pointer-events: none;
  }
</style>
