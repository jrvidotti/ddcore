<script lang="ts">
  // A Markdown Editor stores its source, which is the user's document. The
  // preview is rendered by the server, so what is previewed is exactly what
  // print and the API will render later.
  import { __ } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { sanitizeHtml } from "$lib/richtext";

  let { value, onchange, readOnly = false, error = "", id = "", rows = 8 }:
    { value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string; rows?: number } = $props();

  let previewing = $state(false);
  let html = $state("");
  let rendering = $state(false);
  let renderedFor = "";

  async function render(source: string) {
    if (renderedFor === source) return;
    rendering = true;
    try {
      const res = await api.post<{ html: string }>("/api/richtext/markdown", { text: source });
      html = res.html || "";
      renderedFor = source;
    } catch {
      // a preview that cannot be rendered says so instead of showing the last
      // one as if it were current
      html = "";
      renderedFor = "";
    } finally {
      rendering = false;
    }
  }

  function showPreview() {
    previewing = true;
    render(String(value ?? ""));
  }
</script>

<div class="markdown" class:error={!!error}>
  <div class="tabs">
    <button type="button" class:on={!previewing} onclick={() => (previewing = false)} disabled={readOnly}>{__("Write")}</button>
    <button type="button" class:on={previewing} onclick={showPreview}>{__("Preview")}</button>
  </div>
  {#if previewing || readOnly}
    <div class="preview">
      {#if rendering}<span class="muted small">{__("Loading…")}</span>
      {:else if html}{@html sanitizeHtml(html)}
      {:else}<span class="muted small">—</span>{/if}
    </div>
  {:else}
    <textarea
      {id}
      class="input md"
      class:error={!!error}
      readonly={readOnly}
      {rows}
      spellcheck="true"
      value={value ?? ""}
      placeholder={__("Markdown")}
      onchange={(e) => onchange((e.target as HTMLTextAreaElement).value || null)}
    ></textarea>
  {/if}
</div>

<style>
  .markdown { border: 1px solid var(--border); border-radius: 6px; overflow: hidden; background: var(--bg-input, #fff); }
  .markdown.error { border-color: var(--danger, #dc2626); }
  .tabs { display: flex; gap: 2px; padding: 4px; border-bottom: 1px solid var(--border); background: var(--bg-subtle, #f8fafc); }
  .tabs button {
    border: 0;
    border-radius: 4px;
    padding: 3px 10px;
    background: transparent;
    font-size: 12px;
    color: var(--text-muted, #64748b);
    cursor: pointer;
  }
  .tabs button.on { background: var(--bg-hover, #e2e8f0); color: var(--text, #0f172a); }
  textarea.md {
    border: 0;
    border-radius: 0;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
    line-height: 1.55;
  }
  .preview { padding: 8px 10px; font-size: 13px; line-height: 1.55; min-height: 80px; }
  .preview :global(p) { margin: 0 0 8px; }
  .preview :global(:last-child) { margin-bottom: 0; }
  .preview :global(table) { border-collapse: collapse; }
  .preview :global(th), .preview :global(td) { border: 1px solid var(--border); padding: 3px 6px; }
  .preview :global(pre) { background: var(--bg-subtle, #f1f5f9); padding: 8px; border-radius: 4px; overflow-x: auto; }
</style>
