<script lang="ts">
  import { onDestroy } from "svelte";
  import { NotificationCenter } from "$lib/notification-center.svelte";
  import { __, doctypeLabel, siteName } from "$lib/boot.svelte";
  import { formatDatetime } from "$lib/format";
  import { notifications, refreshNotifications } from "$lib/notifications.svelte";
  import Icon from "$lib/components/Icon.svelte";

  const center = new NotificationCenter();
  $effect(() => { notifications.revision; void center.load(center.offset, center.filter); });
  onDestroy(() => center.destroy());
</script>

<svelte:head><title>{__("Notifications")} · {siteName()}</title></svelte:head>
<div class="page notifications">
  <div class="page-head">
    <h1>{__("Notifications")}</h1>
    <button class="btn" onclick={() => refreshNotifications()} disabled={center.loading}><Icon name="refresh-cw" />{__("Refresh")}</button>
  </div>
  <div class="toolbar">
    <label for="notification-filter">{__("Show")}</label>
    <select id="notification-filter" bind:value={center.filter} onchange={() => { center.offset = 0; }}>
      <option value="all">{__("All")}</option>
      <option value="unread">{__("Unread")}</option>
      <option value="read">{__("Read")}</option>
    </select>
  </div>
  <section aria-label={__("Notifications")} aria-busy={center.loading}>
    {#if center.error}
      <div class="state" role="alert"><p>{center.error}</p><button class="btn" onclick={() => center.load(center.offset, center.filter)}>{__("Retry")}</button></div>
    {:else if center.loading}
      <p class="state muted" role="status">{__("Loading notifications…")}</p>
    {:else if center.rows.length === 0}
      <div class="state muted"><Icon name="bell" size={28} /><p>{__("No notifications")}</p></div>
    {:else}
      <ul>
        {#each center.rows as row (row.id)}
          <li class:unread={!row.read}>
            <div class="content">
              <div class="heading"><h2>{row.title}</h2><span class="status">{row.read ? __("Read") : __("Unread")}</span></div>
              <p class="message">{row.message}</p>
              <div class="metadata"><time datetime={row.creation}>{formatDatetime(row.creation)}</time><span>{doctypeLabel(row.reference_doctype)} · {row.reference_id}</span></div>
            </div>
            <div class="actions">
              <button class="btn" disabled={center.pending !== ""} onclick={() => center.open(row)}><Icon name="external-link" />{__("Open document")}</button>
              <button class="btn icon" disabled={center.pending !== ""} onclick={() => center.toggle(row)} title={row.read ? __("Mark as unread") : __("Mark as read")} aria-label={row.read ? __("Mark as unread") : __("Mark as read")}><Icon name={row.read ? "mail" : "check"} /></button>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
  <div class="pagination">
    <span class="muted" aria-live="polite">{__("{0} notifications", [center.total])}</span>
    <button class="btn" disabled={center.loading || center.offset === 0} onclick={() => { center.offset = Math.max(0, center.offset - center.limit); }}>{__("Previous")}</button>
    <button class="btn" disabled={center.loading || center.offset + center.limit >= center.total} onclick={() => { center.offset += center.limit; }}>{__("Next")}</button>
  </div>
</div>

<style>
  .notifications { max-width: 1100px; }
  .page-head, .toolbar, .pagination, .heading, .metadata, .actions { display: flex; align-items: center; gap: 12px; }
  .page-head { justify-content: space-between; margin-bottom: 24px; }
  h1 { margin: 0; }
  .toolbar { margin-bottom: 16px; }
  select { width: auto; min-width: 130px; }
  section { border: 1px solid var(--border); border-radius: 8px; background: white; overflow: hidden; }
  ul { list-style: none; margin: 0; padding: 0; }
  li { display: flex; align-items: center; gap: 20px; padding: 20px; border-bottom: 1px solid var(--border); }
  li:last-child { border-bottom: 0; }
  li.unread { background: #f7faff; box-shadow: inset 3px 0 var(--primary); }
  .content { flex: 1; min-width: 0; overflow-wrap: anywhere; }
  h2 { margin: 0; font-size: 14px; font-weight: 600; }
  .status { font-size: 11px; color: var(--muted); flex-shrink: 0; }
  .unread .status { color: var(--primary); }
  li:not(.unread) .content { color: var(--muted); opacity: .75; }
  li:not(.unread) h2 { font-weight: 500; }
  .actions { flex-shrink: 0; gap: 8px; }
  .message { margin: 8px 0 12px; white-space: pre-wrap; }
  .metadata { flex-wrap: wrap; font-size: 12px; color: var(--muted); }
  .state { padding: 44px 20px; text-align: center; }
  .pagination { margin-top: 16px; justify-content: flex-end; }
  .pagination span { margin-right: auto; }
  @media (max-width: 600px) { li { align-items: flex-start; flex-direction: column; gap: 12px; } .heading { align-items: flex-start; } }
</style>
