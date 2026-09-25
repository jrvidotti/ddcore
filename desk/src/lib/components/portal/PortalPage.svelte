<script lang="ts">
  // One portal page (OPS-10): its list, its record, a new document or one
  // document on it, depending on `id` ("new" opens the creation form).
  import { goto } from "$app/navigation";
  import { __ } from "$lib/boot.svelte";
  import { showError } from "$lib/ui.svelte";
  import { registerTitles } from "$lib/titles.svelte";
  import { portalApi, portalHref, type PortalPageMeta, type PortalRows } from "$lib/portal";
  import PortalForm from "./PortalForm.svelte";
  import PortalList from "./PortalList.svelte";
  import Spinner from "../Spinner.svelte";

  let { portal, page, id = undefined }: { portal: string; page: string; id?: string } = $props();

  let meta = $state<PortalPageMeta | null>(null);
  let doc = $state<Record<string, any> | null>(null);
  let rows = $state<PortalRows | null>(null);
  let missing = $state(false);
  let version = 0;

  $effect(() => { void load(portal, page, id); });

  async function load(portal: string, page: string, id?: string) {
    const v = ++version;
    meta = null; doc = null; rows = null; missing = false;
    try {
      const m = await portalApi.page(portal, page);
      if (v !== version) return;
      if (id === "new") {
        const d = await portalApi.defaults(portal, page);
        if (d?._linkTitles) registerTitles(d._linkTitles);
        doc = d || {};
      } else if (id || m.page.kind === "record") {
        doc = await portalApi.get(portal, page, id);
      } else {
        rows = await portalApi.list(portal, page);
        registerTitles(rows.titles);
      }
      if (v === version) meta = m;
    } catch (e: any) {
      if (v !== version) return;
      if (e?.status === 404 && !id) missing = true;
      else showError(e);
    }
  }

  async function more() {
    if (!rows || !meta) return;
    const next = await portalApi.list(portal, page, rows.rows.length);
    registerTitles(next.titles);
    rows = { rows: [...rows.rows, ...next.rows], titles: { ...rows.titles, ...next.titles }, more: next.more };
  }

  function saved(d: Record<string, any>) {
    if (!meta) return;
    if (id === "new" || meta.page.kind === "list") goto(portalHref(portal, page, d.id), { replaceState: id === "new" });
    else doc = d;
  }
</script>

<div class="page portal-page">
  {#if missing}
    <div class="card empty">{__("There is nothing here for you yet.")}</div>
  {:else if meta}
    <div class="page-head">
      <div>
        {#if id && meta.page.kind === "list"}
          <a class="small back" href={portalHref(portal, page)}>← {meta.page.label}</a>
        {/if}
        <h1>{id === "new" ? __("New {0}", [meta.page.doctypeLabel]) : id && meta.page.kind === "list" ? (doc?.[meta.page.titleField || ""] || doc?.id) : meta.page.label}</h1>
        {#if meta.page.description && !id}<p class="muted small">{meta.page.description}</p>{/if}
      </div>
      <div class="spacer"></div>
      {#if !id}
        {#each meta.page.actions as a}
          <a class="btn" href={portalHref(portal, a.page, a.new ? "new" : undefined)}>{a.label}</a>
        {/each}
        {#if meta.page.kind === "list" && meta.page.create}
          <a class="btn primary" href={portalHref(portal, page, "new")}>{__("New")}</a>
        {/if}
      {/if}
    </div>
    {#if rows}
      <PortalList {meta} {portal} data={rows} onmore={more} />
    {:else if doc}
      <PortalForm {meta} {portal} {doc} isNew={id === "new"} onsaved={saved} />
    {/if}
  {:else}
    <Spinner />
  {/if}
</div>

<style>
  .portal-page { max-width: 960px; margin: 0 auto; }
  .page-head { display: flex; align-items: flex-end; gap: 8px; flex-wrap: wrap; margin-bottom: 16px; }
  .page-head h1 { margin: 2px 0 0; }
  .back { color: var(--muted); }
  .empty { padding: 24px; text-align: center; color: var(--muted); }
</style>
