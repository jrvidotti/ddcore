<script lang="ts">
  // The account screen, without API keys: a portal user has no API of their own.
  import Profile from "$lib/components/Profile.svelte";
</script>

<Profile apiKeys={false} />
