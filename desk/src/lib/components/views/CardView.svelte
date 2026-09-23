<script lang="ts">
  import { __, doctypeLabel } from "$lib/boot.svelte";
  import type { ListViewOptions } from "$lib/desk-sdk";
  import type { Field, Meta } from "$lib/meta";
  import { formatValue, statusColor } from "$lib/format";
  import { getLinkTitle } from "$lib/titles.svelte";
  import { SvelteSet } from "svelte/reactivity";
  import { resolveCardFields } from "./card-fields";
  import { avatarColor, initials } from "./card-avatar";

  let { rows, meta, doctype, wsPrefix, selected, settings = {}, onToggle, cellText, loading = false }: {
    rows: any[]; meta: Meta; doctype: string; wsPrefix: string; selected: Set<string>;
    settings?: ListViewOptions; onToggle: (id: string) => void;
    cellText?: (row: any, field: Field) => string; loading?: boolean;
  } = $props();
  const card = $derived(settings.card || {});
  const cardInfo = $derived(resolveCardFields(meta.doctype, settings.card));
  const statusField = $derived(meta.doctype.fields.find((f) => f.fieldname === "status"));
  // images that failed to load, by row and URL, so a new upload is tried again
  const failed = new SvelteSet<string>();
  function imageSrc(row: any): string {
    const src = cardInfo.image ? row[cardInfo.image] : "";
    return src && !failed.has(`${row.id}\n${src}`) ? String(src) : "";
  }

  function text(row: any, name: string) {
    const field = meta.doctype.fields.find((f) => f.fieldname === name);
    const formatter = settings.formatters?.[name];
    if (formatter) return formatter(row[name], row);
    if (field?.fieldtype === "Dynamic Link") return getLinkTitle(row[field.options], row[name]) || String(row[name] ?? "");
    if (field && cellText) return cellText(row, field);
    if (name === "ref_doctype" || name === "reference_doctype" || name.endsWith("_doctype")) return row[name] ? doctypeLabel(row[name]) : "";
    return formatValue(row[name], field);
  }
  function indicator(row: any) {
    const custom = card.indicator || settings.indicator;
    if (custom) return custom(row);
    if (statusField && row[statusField.fieldname!]) return { label: formatValue(row[statusField.fieldname!], statusField), color: statusColor(row[statusField.fieldname!], statusField) };
    if (meta.doctype.submittable) {
      const status = Number(row.docstatus) === 2 ? "Cancelled" : Number(row.docstatus) === 1 ? "Submitted" : "Draft";
      return { label: __(status), color: statusColor(status) };
    }
    return null;
  }
</script>

<div class="cards">
  {#each rows as row (row.id)}
    {@const ind = indicator(row)}
    {@const badges = (card.badges || settings.badges)?.(row) || []}
    {@const title = text(row, cardInfo.title) || row.id}
    {@const src = imageSrc(row)}
    <article class="card record" class:selected={selected.has(row.id)} class:with-media={cardInfo.hasImage}>
      <header>
        <input type="checkbox" aria-label={__("Select {0}", [row.id])} checked={selected.has(row.id)} onclick={(e) => e.stopPropagation()} onchange={() => onToggle(row.id)} />
        {#if cardInfo.hasImage}
          {#if src}
            <img class="media" {src} alt="" loading="lazy" decoding="async" onerror={() => failed.add(`${row.id}\n${src}`)} />
          {:else}
            <span class="media card-avatar {avatarColor(title)}" aria-hidden="true">{initials(title)}</span>
          {/if}
        {/if}
        <div class="heading">
          <a class="title" href={`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(row.id)}`}>{title}</a>
          {#if cardInfo.hasImage && cardInfo.subtitle}<div class="subtitle muted">{text(row, cardInfo.subtitle)}</div>{/if}
        </div>
        {#if ind}<span class="indicator {ind.color}">{ind.label}</span>{/if}
      </header>
      {#if !cardInfo.hasImage && cardInfo.subtitle}<div class="subtitle muted">{text(row, cardInfo.subtitle)}</div>{/if}
      {#if cardInfo.keyFields.length}
        <dl>{#each cardInfo.keyFields as field}<div><dt>{field.label}</dt><dd>{text(row, field.fieldname!)}</dd></div>{/each}</dl>
      {/if}
      {#if cardInfo.dateField || badges.length}
        <footer>
          {#if cardInfo.dateField}<span class="muted small">{text(row, cardInfo.dateField)}</span>{/if}
          <span class="badges">{#each badges as badge (badge.label)}<span class="indicator {badge.color}">{badge.label}</span>{/each}</span>
        </footer>
      {/if}
    </article>
  {/each}
</div>
{#if !loading && !rows.length}<div class="card empty">{__("No records")}</div>{/if}

<style>
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 12px; }
  .record { position: relative; min-width: 0; display: flex; flex-direction: column; padding: 16px; }
  .record:hover, .record:focus-within { border-color: var(--primary); }
  .record.selected { border-color: var(--primary); }
  header { display: flex; align-items: flex-start; gap: 10px; }
  header input { position: relative; z-index: 1; flex-shrink: 0; width: 18px; height: 18px; margin: 1px 0 0; cursor: pointer; }
  .heading { flex: 1; min-width: 0; }
  .title { display: block; font-weight: 600; color: var(--text); overflow-wrap: anywhere; }
  .media { flex-shrink: 0; width: 44px; height: 44px; border-radius: var(--radius); }
  img.media { object-fit: cover; border: 1px solid var(--border); background: var(--border); }
  .card-avatar { display: inline-flex; align-items: center; justify-content: center; font-weight: 600; font-size: 16px; letter-spacing: .02em; user-select: none; }
  .title::after { content: ""; position: absolute; inset: 0; border-radius: var(--radius); }
  .title:focus { outline: none; }
  .title:focus-visible::after { outline: 2px solid var(--primary); outline-offset: 2px; }
  header .indicator { flex-shrink: 0; max-width: 45%; overflow-wrap: anywhere; }
  .subtitle { margin: 8px 0 0 28px; overflow-wrap: anywhere; }
  .with-media .subtitle { margin: 4px 0 0; }
  dl { display: grid; gap: 8px; margin: 16px 0; }
  dl > div { display: flex; justify-content: space-between; gap: 12px; }
  dt { color: var(--muted); font-size: 12px; }
  dd { margin: 0; text-align: right; overflow-wrap: anywhere; min-width: 0; }
  footer { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: 8px; margin-top: auto; padding-top: 12px; border-top: 1px solid var(--border); }
  .badges { display: inline-flex; flex-wrap: wrap; gap: 4px; }
  @media (max-width: 360px) { .cards { grid-template-columns: minmax(0, 1fr); } }
</style>
