<script lang="ts">
  // The user's own rows on a list page (OPS-10).
  import { formatValue, statusColor } from "$lib/format";
  import { __ } from "$lib/boot.svelte";
  import { portalHref, type PortalPageMeta, type PortalRows } from "$lib/portal";

  let { meta, portal, data, onmore = undefined }: {
    meta: PortalPageMeta; portal: string; data: PortalRows; onmore?: () => void;
  } = $props();

  const columns = $derived(meta.page.listFields.map((n) => meta.fields.find((f) => f.fieldname === n)).filter((f) => !!f));

  function cell(row: Record<string, any>, f: any): string {
    const v = row[f.fieldname];
    if ((f.fieldtype === "Link") && v) return data.titles?.[f.options]?.[v] || v;
    if ((f.fieldtype === "Attach" || f.fieldtype === "Attach Image") && v) return __("Attached");
    return formatValue(v, f);
  }
</script>

{#if !data.rows.length}
  <div class="card empty">{__("Nothing here yet.")}</div>
{:else}
  <div class="card list">
    <table>
      <thead>
        <tr>
          {#each columns as f}<th>{f!.label}</th>{/each}
          {#if meta.page.stateField}<th>{__("Status")}</th>{/if}
        </tr>
      </thead>
      <tbody>
        {#each data.rows as row (row.id)}
          <tr>
            {#each columns as f, i}
              <td>
                {#if i === 0}
                  <a href={portalHref(portal, meta.page.name, row.id)}>{cell(row, f) || row.id}</a>
                {:else}
                  {cell(row, f)}
                {/if}
              </td>
            {/each}
            {#if meta.page.stateField}
              <td>{#if row[meta.page.stateField]}<span class="indicator {statusColor(row[meta.page.stateField])}">{__(row[meta.page.stateField])}</span>{/if}</td>
            {/if}
          </tr>
        {/each}
      </tbody>
    </table>
  </div>
  {#if data.more && onmore}
    <div class="more"><button class="btn" onclick={onmore}>{__("Load more")}</button></div>
  {/if}
{/if}

<style>
  .list { overflow-x: auto; }
  table { width: 100%; border-collapse: collapse; font-size: 14px; }
  th { text-align: left; font-weight: 500; font-size: 12px; color: var(--muted); padding: 10px 14px; border-bottom: 1px solid var(--border); }
  td { padding: 10px 14px; border-bottom: 1px solid var(--border); }
  tr:last-child td { border-bottom: 0; }
  .empty { padding: 24px; text-align: center; color: var(--muted); }
  .more { display: flex; justify-content: center; margin-top: 12px; }
</style>
