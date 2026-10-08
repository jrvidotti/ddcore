<script lang="ts">
  // A Select value as an indicator: its colour (optionColors), its icon
  // (optionIcons) in place of the dot, and its translated label — or, with
  // iconOnly and an icon, the icon alone and the label as the tooltip.
  import type { Field } from "$lib/meta";
  import { selectIndicator } from "$lib/format";
  import Icon from "./Icon.svelte";
  let { value, field, iconOnly = false }: { value: unknown; field?: Partial<Field>; iconOnly?: boolean } = $props();
  const ind = $derived(selectIndicator(value, field));
  const bare = $derived(iconOnly && !!ind?.icon);
</script>

{#if ind}
  <span class="indicator select-indicator {ind.color}" class:has-icon={!!ind.icon} class:icon-only={bare} title={ind.label} aria-label={bare ? ind.label : undefined} role={bare ? "img" : undefined}>
    {#if ind.icon}<Icon name={ind.icon} size={12} />{/if}
    {#if !bare}<span class="indicator-label">{ind.label}</span>{/if}
  </span>
{/if}
