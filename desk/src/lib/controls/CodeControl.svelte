<script lang="ts">
  // A Code field: a monospace box that keeps every byte, including the leading
  // whitespace that carries meaning in what is being written.
  import { __ } from "$lib/boot.svelte";

  let {
    value, onchange, readOnly = false, error = "", id = "", language = "", rows = 8,
  }: {
    value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string;
    language?: string; rows?: number;
  } = $props();

  // Tab indents instead of leaving the field: in a code box, leaving is what
  // the mouse is for.
  function onkeydown(e: KeyboardEvent) {
    if (e.key !== "Tab" || e.ctrlKey || e.metaKey) return;
    e.preventDefault();
    const el = e.target as HTMLTextAreaElement;
    const { selectionStart: start, selectionEnd: end, value: text } = el;
    if (e.shiftKey) {
      const lineStart = text.lastIndexOf("\n", start - 1) + 1;
      const indent = text.slice(lineStart, lineStart + 2) === "  " ? 2 : text[lineStart] === "\t" ? 1 : 0;
      if (!indent) return;
      el.value = text.slice(0, lineStart) + text.slice(lineStart + indent);
      el.selectionStart = el.selectionEnd = Math.max(lineStart, start - indent);
    } else {
      el.value = text.slice(0, start) + "  " + text.slice(end);
      el.selectionStart = el.selectionEnd = start + 2;
    }
    onchange(el.value || null);
  }
</script>

<div class="code-box" class:error={!!error}>
  {#if language}<span class="lang">{language}</span>{/if}
  <textarea
    {id}
    class="input code"
    class:error={!!error}
    readonly={readOnly}
    {rows}
    spellcheck="false"
    autocapitalize="off"
    value={value ?? ""}
    placeholder={readOnly ? "" : __("Code")}
    {onkeydown}
    onchange={(e) => onchange((e.target as HTMLTextAreaElement).value || null)}
  ></textarea>
</div>

<style>
  .code-box { position: relative; }
  .lang {
    position: absolute;
    top: 4px;
    right: 8px;
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-muted, #64748b);
    pointer-events: none;
  }
  textarea.code {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
    line-height: 1.5;
    tab-size: 2;
    white-space: pre;
    overflow-wrap: normal;
    overflow-x: auto;
  }
</style>
