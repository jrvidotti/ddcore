<script lang="ts">
  // A portal without a page named opens its first page.
  import { goto } from "$app/navigation";
  import { page } from "$app/state";
  import { boot } from "$lib/boot.svelte";
  import { portalHref } from "$lib/portal";

  $effect(() => {
    const p = (boot.data?.portals || []).find((x) => x.slug === page.params.portal);
    goto(p?.pages.length ? portalHref(p.slug, p.pages[0].name) : "/portal", { replaceState: true });
  });
</script>
