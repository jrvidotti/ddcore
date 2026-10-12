<script lang="ts">
  import { siteLogo, siteLogoImage } from "$lib/boot.svelte";

  /**
   * The site's mark (#128): the logo image its app declares, or the letter
   * square of `desk.logo` in the theme's primary colour. `size` is its height;
   * an image keeps its proportions. `alt` names the site where its name is not
   * written beside the mark, and stays empty where it is.
   */
  let { size = 26, alt = "", class: cls = "" }: { size?: number; alt?: string; class?: string } = $props();
  const image = $derived(siteLogoImage());
</script>

{#if image}
  <img class="site-mark image {cls}" src={image} {alt} style:height="{size}px" style:max-width="{size * 5}px" />
{:else}
  <span
    class="site-mark letter {cls}"
    role={alt ? "img" : undefined}
    aria-label={alt || undefined}
    aria-hidden={alt ? undefined : "true"}
    style:width="{size}px"
    style:height="{size}px"
    style:border-radius="{Math.round(size / 4)}px"
    style:font-size="{Math.round(size / 2)}px">{siteLogo()}</span>
{/if}

<style>
  .site-mark { flex-shrink: 0; }
  .image { display: block; width: auto; object-fit: contain; }
  .letter { display: inline-flex; align-items: center; justify-content: center; background: var(--primary); color: var(--on-primary); font-weight: 700; overflow: hidden; }
</style>
