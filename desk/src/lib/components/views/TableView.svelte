<script lang="ts">
  import { goto } from "$app/navigation";
  import { __ } from "$lib/boot.svelte";
  import type { ListViewOptions } from "$lib/desk-sdk";
  import type { Field, Meta } from "$lib/meta";
  import { statusColor, timeAgo } from "$lib/format";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { showNameColumn } from "./name-column";

  let {
    rows, meta, doctype, wsPrefix, columns, selected, orderBy, loading = false,
    settings = {}, onSort, onToggle, onSelectAll, cellText, statusOf, statusLabelOf,
  }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; columns: Field[];
    selected: Set<string>; orderBy: string; loading?: boolean; settings?: ListViewOptions;
    onSort: (field: Field) => void; onToggle: (name: string) => void;
    onSelectAll: (checked: boolean) => void; cellText: (row: any, field: Field) => string;
    statusOf: (row: any) => string; statusLabelOf: (row: any) => string;
  } = $props();

  const statusField = $derived(meta.doctype.fields.find((f) => f.fieldname === "status"));
  const showIndicatorColumn = $derived(!!settings.indicator ||
    (!columns.some((c) => c.fieldname === statusField?.fieldname) && (!!statusField || !!meta.doctype.submittable)));
  const showName = $derived(showNameColumn(meta.doctype, columns, settings));
  const num = (f: Field) => ["Int", "Float", "Currency", "Percent"].includes(f.fieldtype);
  const documentUrl = (name: string) => `${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`;
</script>

<table class="grid">
  <thead>
    <tr>
      <th style="width:28px"><input type="checkbox" aria-label={__("Select all")} checked={rows.length > 0 && rows.every((r) => selected.has(r.name))} onchange={(e) => onSelectAll(e.currentTarget.checked)} /></th>
      {#if showName}
        <th aria-sort={orderBy.startsWith("name ") ? (orderBy.endsWith("asc") ? "ascending" : "descending") : "none"}>
          <button class="sort" onclick={() => onSort({ fieldname: "name", fieldtype: "Data" })}>{meta.doctype.nameLabel || __("Name")} {#if orderBy.startsWith("name ")}{orderBy.endsWith("asc") ? "↑" : "↓"}{/if}</button>
        </th>
      {/if}
      {#each columns as c}
        <th class:num={num(c)} aria-sort={orderBy.startsWith(c.fieldname + " ") ? (orderBy.endsWith("asc") ? "ascending" : "descending") : "none"}>
          <button class="sort" onclick={() => onSort(c)}>{c.label} {#if orderBy.startsWith(c.fieldname + " ")}{orderBy.endsWith("asc") ? "↑" : "↓"}{/if}</button>
        </th>
      {/each}
      {#if showIndicatorColumn}<th>{__("Status")}</th>{/if}
      {#if settings.modifiedColumn !== false}<th class="num">{__("Modified")}</th>{/if}
    </tr>
  </thead>
  <tbody>
    {#each rows as r (r.name)}
      <tr class="row" style="cursor:pointer" tabindex="0" onclick={() => goto(documentUrl(r.name))} onkeydown={(e) => { if (e.target === e.currentTarget && (e.key === "Enter" || e.key === " ")) { e.preventDefault(); goto(documentUrl(r.name)); } }}>
        <td onclick={(e) => { e.stopPropagation(); onToggle(r.name); }}><input type="checkbox" aria-label={__("Select {0}", [r.name])} checked={selected.has(r.name)} onclick={(e) => e.stopPropagation()} onchange={() => onToggle(r.name)} /></td>
        {#if showName}<td><a href={documentUrl(r.name)} onclick={(e) => e.stopPropagation()}>{r.name}</a></td>{/if}
        {#each columns as c}
          <td class:num={num(c)} class:bold={c.bold} style:font-weight={c.bold ? 600 : undefined}>
            {#if (c.fieldtype === "Link" || c.fieldtype === "Dynamic Link") && r[c.fieldname!]}
              {@const linkTarget = c.fieldtype === "Dynamic Link" ? r[c.options] : c.options}
              {@const linkVal = r[c.fieldname!]}
              <a href={`${wsPrefix}/${encodeURIComponent(linkTarget)}/${encodeURIComponent(linkVal)}`} title={linkVal} onclick={(e) => e.stopPropagation()}>{getLinkTitle(linkTarget, linkVal) || linkVal}</a>
            {:else if !showName && c.fieldname === meta.doctype.titleField}
              <a href={documentUrl(r.name)} onclick={(e) => e.stopPropagation()}>{cellText(r, c) || r.name}</a>
            {:else if c.fieldname === statusField?.fieldname}
              <span class="badges"><span class="indicator {statusColor(r[c.fieldname!], c)}">{__(r[c.fieldname!])}</span>{#if !showIndicatorColumn}{@render badges(r)}{/if}</span>
            {:else}{cellText(r, c)}{/if}
          </td>
        {/each}
        {#if showIndicatorColumn}
          <td><span class="badges">
            {#if settings.indicator}
              {@const ind = settings.indicator(r)}
              {#if ind}<span class="indicator {ind.color}">{ind.label}</span>{/if}
            {:else if statusOf(r)}<span class="indicator {statusColor(statusOf(r), statusField)}">{statusLabelOf(r)}</span>{/if}
            {@render badges(r)}
          </span></td>
        {/if}
        {#if settings.modifiedColumn !== false}<td class="num muted small">{timeAgo(r.modified)}</td>{/if}
      </tr>
    {/each}
    {#if !loading && !rows.length}<tr><td colspan="20" class="empty">{__("No records")}</td></tr>{/if}
  </tbody>
</table>

{#snippet badges(row: any)}
  {#each settings.badges?.(row) || [] as badge (badge.label)}<span class="indicator {badge.color}">{badge.label}</span>{/each}
{/snippet}

<style>
  .badges { display: inline-flex; flex-wrap: wrap; gap: 4px; }
  .sort { border: 0; padding: 0; background: none; color: inherit; font: inherit; cursor: pointer; }
  .sort:focus-visible { outline: 2px solid var(--primary); outline-offset: 3px; }
  .row:focus-visible { outline: 2px solid var(--primary); outline-offset: -2px; }
</style>
