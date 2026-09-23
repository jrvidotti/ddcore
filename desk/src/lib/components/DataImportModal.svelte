<script lang="ts">
  import { focusTrap } from "$lib/focus-trap";
  // Data Import: pick a CSV or XLSX file, see what a dry run makes of it —
  // which column goes to which field, and which rows would fail and why — and
  // only then write it. The server keeps nothing between the two requests:
  // the same File is posted again for the real run.
  import { api, type DataImportResult } from "$lib/api";
  import { __ } from "$lib/boot.svelte";
  import { downloadCsv } from "$lib/csv";
  import { confirm, showError, toast } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";
  import { errorRowsCsv, importModes, importOptionsFromLocale, templateUrl } from "./data-import";

  let {
    open = false,
    doctype,
    label,
    permissions,
    onclose,
    onimported,
  }: {
    open: boolean;
    doctype: string;
    label: string;
    permissions: Record<string, boolean> | undefined;
    onclose: () => void;
    onimported: () => void;
  } = $props();

  /** How many failed rows the dialog lists; the download has them all. */
  const SHOWN_ERRORS = 100;

  const modes = $derived(importModes(permissions));
  let mode = $state<"insert" | "update">("insert");
  let file = $state<File | null>(null);
  let checked = $state<DataImportResult | null>(null);
  let done = $state<DataImportResult | null>(null);
  let choice = $state<Record<string, string>>({});
  /** The columns the person remapped by hand, kept across re-checks. */
  let overrides = $state<Record<string, string>>({});
  let busy = $state(false);
  // how the file writes dates and numbers: the desk's locale unless the
  // person says the file came from somewhere else
  const fromLocale = importOptionsFromLocale();
  let dateOrder = $state<"dmy" | "mdy" | "ymd">(fromLocale.dateOrder);
  let decimal = $state<"." | ",">(fromLocale.decimal);
  let input = $state<HTMLInputElement | null>(null);

  $effect(() => {
    if (open && modes.length && !modes.includes(mode)) mode = modes[0];
  });

  const shown = $derived(done ?? checked);
  const failed = $derived((shown?.rows ?? []).filter((r) => r.status === "error"));
  const good = $derived(shown ? shown.counts.inserted + shown.counts.updated : 0);

  function reset() {
    file = null; checked = null; done = null; choice = {}; overrides = {};
    if (input) input.value = "";
  }

  function close() {
    reset();
    onclose();
  }

  async function run(dryRun: boolean) {
    if (!file) return null;
    busy = true;
    try {
      return await api.dataImport(doctype, file, {
        mode, dryRun, dateOrder, decimal, columns: overrides,
      });
    } catch (e) {
      showError(e);
      return null;
    } finally {
      busy = false;
    }
  }

  async function check() {
    done = null;
    const res = await run(true);
    checked = res;
    if (res) {
      const c: Record<string, string> = {};
      for (const col of res.columns) c[col.header] = col.status === "mapped" ? col.fieldname ?? "" : "";
      choice = c;
    }
  }

  function pick(e: Event) {
    file = (e.currentTarget as HTMLInputElement).files?.[0] ?? null;
    checked = null; choice = {}; overrides = {};
    if (file) check();
  }

  function setMode(m: "insert" | "update") {
    mode = m;
    checked = null; choice = {}; overrides = {};
    if (file) check();
  }

  function remap(header: string, fieldname: string) {
    choice = { ...choice, [header]: fieldname };
    overrides = { ...overrides, [header]: fieldname };
    check();
  }

  async function load() {
    if (!checked || !good) return;
    if (checked.counts.errors && !(await confirm(__("{0} rows have errors and will be skipped. Import the other {1}?", [checked.counts.errors, good])))) return;
    const res = await run(false);
    if (!res) return;
    done = res;
    onimported();
    const n = res.counts.inserted + res.counts.updated;
    toast(__("{0} rows imported", [n]), { indicator: res.counts.errors ? "orange" : "green" });
  }

  function downloadErrors() {
    if (!shown) return;
    const base = (shown.file.name || doctype).replace(/\.[^.]+$/, "");
    downloadCsv(`${base}-errors.csv`, errorRowsCsv(shown, __("Error")));
  }

  function fieldOptions(res: DataImportResult) {
    const opts = res.fields.map((f) => ({ value: f.fieldname, label: f.label }));
    if (res.mode === "update" || res.columns.some((c) => c.fieldname === "id")) opts.unshift({ value: "id", label: __("ID") });
    return opts;
  }
</script>

{#if open}
  <div class="modal-bg" role="dialog" aria-modal="true" tabindex="-1" use:focusTrap
    onclick={(e) => e.target === e.currentTarget && !busy && close()}
    onkeydown={(e) => e.key === "Escape" && !busy && close()}>
    <div class="modal lg">
      <div class="head">
        <h3 class="title"><Icon name="upload" size={18} />{__("Import {0}", [label])}</h3>
        <button class="btn icon" onclick={close} disabled={busy} aria-label={__("Close")}><Icon name="x" size={16} /></button>
      </div>
      <div class="body">
        {#if !done}
          {#if modes.length > 1}
            <fieldset class="modes">
              <legend class="label">{__("What the file does")}</legend>
              <label class="check"><input type="radio" name="di-mode" checked={mode === "insert"} disabled={busy} onchange={() => setMode("insert")} /> {__("Create new records")}</label>
              <label class="check"><input type="radio" name="di-mode" checked={mode === "update"} disabled={busy} onchange={() => setMode("update")} /> {__("Update existing records")}</label>
            </fieldset>
          {/if}
          <div class="pick">
            <input bind:this={input} type="file" accept=".csv,.txt,.xlsx,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" disabled={busy} onchange={pick} aria-label={__("File")} />
            {#if mode === "insert"}
              <a class="btn sm" href={templateUrl(doctype)} download><Icon name="download" size={14} />{__("Download template")}</a>
            {/if}
          </div>
          <div class="formats">
            <label>{__("Dates")}
              <select class="input" bind:value={dateOrder} disabled={busy} onchange={() => file && check()}>
                <option value="dmy">{__("Day/month/year")}</option>
                <option value="mdy">{__("Month/day/year")}</option>
                <option value="ymd">{__("Year/month/day")}</option>
              </select>
            </label>
            <label>{__("Decimal separator")}
              <select class="input" bind:value={decimal} disabled={busy} onchange={() => file && check()}>
                <option value=",">{__("Comma (1.234,56)")}</option>
                <option value=".">{__("Point (1,234.56)")}</option>
              </select>
            </label>
          </div>
          <p class="muted small hint">
            {#if mode === "insert"}
              {__("A CSV or Excel (.xlsx) file whose first row names the columns. Each following row creates one record.")}
            {:else}
              {__("A CSV or Excel (.xlsx) file with an ID column naming the record each row changes. Export the list as CSV to get one; a blank cell clears the field.")}
            {/if}
          </p>
        {/if}

        {#if busy}
          <p class="muted">{__("Checking…")}</p>
        {/if}

        {#if checked && !done}
          <h4>{__("Columns")}</h4>
          <table class="grid">
            <thead><tr><th>{__("Column in the file")}</th><th>{__("Field")}</th><th></th></tr></thead>
            <tbody>
              {#each checked.columns as col (col.index)}
                <tr>
                  <td>{col.header || __("(no header)")}</td>
                  <td>
                    <select class="input" value={choice[col.header] ?? ""} disabled={busy || !col.header}
                      onchange={(e) => remap(col.header, (e.currentTarget as HTMLSelectElement).value)}>
                      <option value="">{__("Ignore")}</option>
                      {#each fieldOptions(checked) as o (o.value)}<option value={o.value}>{o.label}</option>{/each}
                    </select>
                  </td>
                  <td class="muted small">{col.status === "mapped" ? "" : col.reason || ""}</td>
                </tr>
              {/each}
            </tbody>
          </table>
        {/if}

        {#if shown}
          <div class="summary" class:ok={!shown.counts.errors}>
            {#if done}
              {__("{0} created, {1} updated, {2} with errors.", [shown.counts.inserted, shown.counts.updated, shown.counts.errors])}
            {:else}
              {__("{0} rows read: {1} can be imported, {2} have errors.", [shown.counts.rows, good, shown.counts.errors])}
            {/if}
          </div>
          {#if failed.length}
            <table class="grid errors">
              <thead><tr><th class="num">{__("Row")}</th><th>{__("ID")}</th><th>{__("Error")}</th></tr></thead>
              <tbody>
                {#each failed.slice(0, SHOWN_ERRORS) as r (r.row)}
                  <tr><td class="num">{r.row}</td><td>{r.id || ""}</td><td>{r.message}</td></tr>
                {/each}
              </tbody>
            </table>
            {#if failed.length > SHOWN_ERRORS}<p class="muted small">{__("Showing {0} of {1} rows with errors; download them all below.", [SHOWN_ERRORS, failed.length])}</p>{/if}
          {/if}
        {/if}
      </div>
      <div class="foot">
        {#if failed.length}
          <button class="btn" onclick={downloadErrors}><Icon name="download" size={14} />{__("Download rows with errors")}</button>
        {/if}
        <span class="spacer"></span>
        {#if done}
          <button class="btn" onclick={reset}>{__("Import another file")}</button>
          <button class="btn primary" onclick={close}>{__("Close")}</button>
        {:else}
          <button class="btn" onclick={close} disabled={busy}>{__("Cancel")}</button>
          <button class="btn primary" disabled={busy || !checked || !good} onclick={load}>{__("Import {0} rows", [good])}</button>
        {/if}
      </div>
    </div>
  </div>
{/if}

<style>
  .title { display: flex; align-items: center; gap: 8px; }
  .label { display: block; font-size: 12px; font-weight: 500; margin-bottom: 4px; color: var(--muted); }
  .modes { display: flex; gap: 16px; flex-wrap: wrap; border: 0; padding: 0; margin: 0 0 12px; }
  .modes legend { width: 100%; }
  .check { display: flex; align-items: center; gap: 6px; font-size: 13px; }
  .pick { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
  .hint { margin: 6px 0 0; }
  .formats { display: flex; gap: 12px; flex-wrap: wrap; margin-top: 10px; }
  .formats label { display: flex; flex-direction: column; gap: 4px; font-size: 12px; color: var(--muted); }
  .formats select { width: auto; }
  h4 { margin: 16px 0 6px; font-size: 13px; }
  .grid select { width: 100%; }
  .summary { margin: 14px 0 8px; padding: 8px 12px; border-radius: var(--radius); background: #fff7ed; font-size: 13px; }
  .summary.ok { background: #f0fdf4; }
  .errors { max-height: 320px; }
  .foot .spacer { flex: 1; }
</style>
