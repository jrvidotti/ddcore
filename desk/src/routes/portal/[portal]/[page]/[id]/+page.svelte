<script lang="ts">
  // `new` is the creation form; anything else is a document's id.
  import { page } from "$app/state";
  import PortalPage from "$lib/components/portal/PortalPage.svelte";
</script>

{#key page.params.id}
  <PortalPage portal={page.params.portal || ""} page={page.params.page || ""} id={page.params.id} />
{/key}
