<script lang="ts">
  import { page } from "$app/state";
  import PortalPage from "$lib/components/portal/PortalPage.svelte";
</script>

<PortalPage portal={page.params.portal || ""} page={page.params.page || ""} />
