<script lang="ts">
  import { onMount } from "svelte";
  import { page } from "$app/state";
  import { goto } from "$app/navigation";
  import { __, boot } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { toast } from "$lib/ui.svelte";
  import Icon from "./Icon.svelte";

  let { doctype, name }: { doctype: string; name: string } = $props();

  let formats = $state<{ name: string; label: string; default?: boolean }[]>([]);
  let selectedFormat = $state("standard");
  let letterheads = $state<{ name: string; is_default?: boolean; disabled?: boolean }[]>([]);
  let selectedLetterhead = $state("");
  let selectedLang = $state("pt-BR");

  let loading = $state(true);
  let rendering = $state(false);
  let downloadingPDF = $state(false);
  let error = $state("");
  let htmlContent = $state("");
  let iframeEl = $state<HTMLIFrameElement | null>(null);

  const workspace = $derived(page.params.workspace || "");
  const wsPrefix = $derived(workspace ? `/app/${encodeURIComponent(workspace)}` : "/app");

  const languages = [
    { code: "pt-BR", label: "Português (Brasil)" },
    { code: "en", label: "English" },
    { code: "es", label: "Español" },
  ];

  onMount(() => {
    if (boot.data?.lang) {
      selectedLang = boot.data.lang;
    }
    loadMetadata();
  });

  async function loadMetadata() {
    loading = true;
    error = "";
    try {
      const [fmtList, lhList] = await Promise.all([
        api.print.formats(doctype).catch(() => [{ name: "standard", label: __("Standard"), default: true }]),
        api.print.letterheads().catch(() => []),
      ]);
      formats = fmtList;
      letterheads = lhList;

      const defLh = letterheads.find((l) => l.is_default);
      if (defLh) {
        selectedLetterhead = defLh.name;
      }

      await loadHTML();
    } catch (e: any) {
      error = e.message || String(e);
    } finally {
      loading = false;
    }
  }

  async function loadHTML() {
    rendering = true;
    try {
      const html = await api.print.html(doctype, name, {
        format: selectedFormat,
        letterhead: selectedLetterhead,
        lang: selectedLang,
      });
      htmlContent = html;
    } catch (e: any) {
      error = e.message || String(e);
    } finally {
      rendering = false;
    }
  }

  function handleIframeLoad() {
    if (!iframeEl || !iframeEl.contentWindow) return;
    try {
      const doc = iframeEl.contentWindow.document;
      const h = Math.max(doc.body.scrollHeight, doc.documentElement.scrollHeight);
      if (h > 100) {
        iframeEl.style.height = `${h + 40}px`;
      }
    } catch {
      // Ignored if cross-origin or sandboxed
    }
  }

  function triggerPrint() {
    if (iframeEl && iframeEl.contentWindow) {
      try {
        iframeEl.contentWindow.focus();
        iframeEl.contentWindow.print();
        return;
      } catch {
        // Fall back to window.print
      }
    }
    window.print();
  }

  async function downloadPDF() {
    downloadingPDF = true;
    const url = api.print.pdfUrl(doctype, name, {
      format: selectedFormat,
      letterhead: selectedLetterhead,
      lang: selectedLang,
      download: true,
    });

    try {
      const res = await fetch(url, { credentials: "same-origin" });
      if (res.status === 503) {
        toast(__("PDF generator not configured on server. Please use browser print."), { indicator: "orange" });
        return;
      }
      if (!res.ok) {
        let errMsg = res.statusText;
        try {
          const json = await res.json();
          if (json?.error?.message) errMsg = json.error.message;
        } catch {}
        toast(errMsg, { indicator: "red" });
        return;
      }

      const blob = await res.blob();
      const blobUrl = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = blobUrl;
      a.download = `${doctype}-${name}.pdf`;
      document.body.appendChild(a);
      a.click();
      document.body.removeChild(a);
      URL.revokeObjectURL(blobUrl);
    } catch (e: any) {
      toast(e.message || String(e), { indicator: "red" });
    } finally {
      downloadingPDF = false;
    }
  }

  function goBack() {
    goto(`${wsPrefix}/${encodeURIComponent(doctype)}/${encodeURIComponent(name)}`);
  }
</script>

<div class="print-view">
  <!-- Top Navigation & Controls Bar -->
  <header class="print-header no-print">
    <div class="header-left">
      <button class="btn icon" onclick={goBack} title={__("Back")} aria-label={__("Back")}>
        <Icon name="chevron-left" />
      </button>
      <div class="header-title">
        <span class="muted">{doctype} /</span>
        <strong>{name}</strong>
      </div>
    </div>

    <div class="header-controls">
      <!-- Format Picker -->
      <div class="control-group">
        <label for="print-format">{__("Format")}:</label>
        <select id="print-format" class="select-input" bind:value={selectedFormat} onchange={loadHTML}>
          {#each formats as fmt}
            <option value={fmt.name}>{fmt.label}</option>
          {/each}
        </select>
      </div>

      <!-- Letter Head Picker -->
      <div class="control-group">
        <label for="print-letterhead">{__("Letter Head")}:</label>
        <select id="print-letterhead" class="select-input" bind:value={selectedLetterhead} onchange={loadHTML}>
          <option value="none">{__("None")}</option>
          {#each letterheads as lh}
            <option value={lh.name}>{lh.name} {lh.is_default ? `(${__("Default")})` : ""}</option>
          {/each}
        </select>
      </div>

      <!-- Language Picker -->
      <div class="control-group">
        <label for="print-lang">{__("Language")}:</label>
        <select id="print-lang" class="select-input" bind:value={selectedLang} onchange={loadHTML}>
          {#each languages as lang}
            <option value={lang.code}>{lang.label}</option>
          {/each}
        </select>
      </div>
    </div>

    <div class="header-actions">
      <button class="btn" onclick={downloadPDF} disabled={downloadingPDF || rendering || loading}>
        <Icon name="download" size={14} />
        <span>{__("Download PDF")}</span>
      </button>

      <button class="btn primary" onclick={triggerPrint} disabled={rendering || loading}>
        <Icon name="printer" size={14} />
        <span>{__("Print")}</span>
      </button>
    </div>
  </header>

  <!-- Main Canvas / Preview Stage -->
  <main class="print-stage">
    {#if loading || rendering}
      <div class="loading-overlay">
        <div class="spinner"></div>
        <p class="muted">{__("Rendering print preview...")}</p>
      </div>
    {/if}

    {#if error}
      <div class="error-card">
        <h3>{__("Error generating print format")}</h3>
        <p>{error}</p>
        <button class="btn" onclick={loadMetadata}>{__("Try again")}</button>
      </div>
    {:else}
      <div class="paper-container">
        <iframe
          bind:this={iframeEl}
          srcdoc={htmlContent}
          title={__("Print Preview")}
          class="paper-frame"
          onload={handleIframeLoad}
        ></iframe>
      </div>
    {/if}
  </main>
</div>

<style>
  .print-view {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg-subtle, #f3f4f6);
    overflow: hidden;
  }

  .print-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 16px;
    padding: 8px 16px;
    background: var(--bg, #ffffff);
    border-bottom: 1px solid var(--border, #e5e7eb);
    z-index: 10;
    flex-shrink: 0;
  }

  .header-left {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .header-title {
    font-size: 14px;
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .muted {
    color: var(--text-muted, #6b7280);
  }

  .header-controls {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }

  .control-group {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 13px;
  }

  .control-group label {
    font-weight: 500;
    color: var(--text-muted, #4b5563);
  }

  .select-input {
    padding: 4px 8px;
    font-size: 13px;
    border: 1px solid var(--border, #d1d5db);
    border-radius: 4px;
    background: var(--bg, #ffffff);
    color: var(--text, #111827);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .print-stage {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    align-items: center;
    padding: 24px 16px;
    position: relative;
  }

  .paper-container {
    width: 100%;
    max-width: 820px;
    background: #ffffff;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.08), 0 1px 3px rgba(0, 0, 0, 0.05);
    border-radius: 4px;
    overflow: hidden;
    margin-bottom: 32px;
  }

  .paper-frame {
    width: 100%;
    min-height: 1120px;
    border: none;
    display: block;
    background: #ffffff;
  }

  .loading-overlay {
    position: absolute;
    top: 32px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    z-index: 5;
    background: rgba(255, 255, 255, 0.9);
    padding: 16px 24px;
    border-radius: 8px;
    box-shadow: 0 2px 8px rgba(0, 0, 0, 0.08);
  }

  .spinner {
    width: 24px;
    height: 24px;
    border: 3px solid rgba(0, 0, 0, 0.1);
    border-top-color: var(--primary, #3b82f6);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }

  .error-card {
    background: var(--bg, #ffffff);
    border: 1px solid var(--border, #e5e7eb);
    padding: 24px;
    border-radius: 8px;
    text-align: center;
    max-width: 480px;
    margin-top: 48px;
  }

  .error-card h3 {
    color: var(--red, #ef4444);
    margin-bottom: 8px;
  }

  .error-card p {
    color: var(--text-muted, #6b7280);
    margin-bottom: 16px;
  }

  @media print {
    .no-print {
      display: none !important;
    }
  }
</style>
