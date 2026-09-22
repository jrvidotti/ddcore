<script lang="ts">
  // A Text Editor field. The value is HTML, cleaned by the server on every
  // write; the editor is loaded on demand so a form without rich text does not
  // pay for it.
  import { __ } from "$lib/boot.svelte";
  import { api } from "$lib/api";
  import { showError } from "$lib/ui.svelte";
  import Icon from "$lib/components/Icon.svelte";
  import { isEmptyRichText, normalizeRichText, sanitizeHtml } from "$lib/richtext";

  let {
    value, onchange, readOnly = false, error = "", id = "", doc = {}, fieldname = "",
  }: {
    value: any; onchange: (v: any) => void; readOnly?: boolean; error?: string; id?: string;
    doc?: any; fieldname?: string;
  } = $props();

  let host = $state<HTMLDivElement | null>(null);
  let editor = $state<any>(null);
  let uploading = $state(false);
  /** What the editor last wrote, so a round trip is not read back as an edit. */
  let lastEmitted = "";
  let active = $state<Record<string, boolean>>({});

  const html = $derived(normalizeRichText(String(value ?? "")));

  $effect(() => {
    if (readOnly || !host) return;
    let cancelled = false;
    (async () => {
      const [{ Editor }, { StarterKit }, { Image }] = await Promise.all([
        import("@tiptap/core"),
        import("@tiptap/starter-kit"),
        import("@tiptap/extension-image"),
      ]);
      if (cancelled || !host) return;
      editor = new Editor({
        element: host,
        extensions: [
          StarterKit.configure({
            link: { openOnClick: false, HTMLAttributes: { rel: "nofollow noreferrer noopener", target: "_blank" } },
          }),
          Image.configure({ allowBase64: false }),
        ],
        content: normalizeRichText(String(value ?? "")),
        onUpdate: ({ editor: ed }: any) => {
          const out = ed.getHTML();
          lastEmitted = out;
          onchange(isEmptyRichText(out) ? null : out);
        },
        onSelectionUpdate: ({ editor: ed }: any) => (active = marks(ed)),
        onTransaction: ({ editor: ed }: any) => (active = marks(ed)),
      });
      active = marks(editor);
    })().catch((err) => showError(err));
    return () => {
      cancelled = true;
      editor?.destroy();
      editor = null;
    };
  });

  // The value can change under the editor — a reload, a fetchFrom, another
  // user's save. Writing it back without `emitUpdate` is what keeps a document
  // from looking edited just because it was opened.
  $effect(() => {
    const incoming = html;
    if (!editor || incoming === lastEmitted) return;
    if (editor.getHTML() !== incoming) editor.commands.setContent(incoming, { emitUpdate: false });
  });

  const marks = (ed: any) => ({
    bold: ed.isActive("bold"), italic: ed.isActive("italic"), strike: ed.isActive("strike"),
    code: ed.isActive("code"), h2: ed.isActive("heading", { level: 2 }), h3: ed.isActive("heading", { level: 3 }),
    bullet: ed.isActive("bulletList"), ordered: ed.isActive("orderedList"), quote: ed.isActive("blockquote"),
    link: ed.isActive("link"),
  });

  function setLink() {
    const previous = editor?.getAttributes("link")?.href ?? "";
    const url = window.prompt(__("Link address"), previous);
    if (url === null) return;
    if (!url) { editor?.chain().focus().unsetLink().run(); return; }
    editor?.chain().focus().extendMarkRange("link").setLink({ href: url }).run();
  }

  async function pickImage(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = "";
    if (!file) return;
    uploading = true;
    try {
      const up = await api.upload(file, { doctype: doc?.parenttype || doc?.doctype, docId: doc?.parent || doc?.id, fieldname });
      editor?.chain().focus().setImage({ src: up.file_url, alt: up.file_name }).run();
    } catch (err) {
      showError(err);
    } finally {
      uploading = false;
    }
  }
</script>

{#if readOnly}
  <div class="richtext-read input" {id}>
    {#if html}{@html sanitizeHtml(html)}{:else}<span class="muted">—</span>{/if}
  </div>
{:else}
  <div class="richtext" class:error={!!error}>
    <div class="toolbar">
      <button type="button" class:on={active.bold} title={__("Bold")} onclick={() => editor?.chain().focus().toggleBold().run()}><Icon name="bold" size={14} /></button>
      <button type="button" class:on={active.italic} title={__("Italic")} onclick={() => editor?.chain().focus().toggleItalic().run()}><Icon name="italic" size={14} /></button>
      <button type="button" class:on={active.strike} title={__("Strikethrough")} onclick={() => editor?.chain().focus().toggleStrike().run()}><Icon name="strikethrough" size={14} /></button>
      <span class="sep"></span>
      <button type="button" class:on={active.h2} title={__("Heading")} onclick={() => editor?.chain().focus().toggleHeading({ level: 2 }).run()}><Icon name="heading-2" size={14} /></button>
      <button type="button" class:on={active.h3} title={__("Subheading")} onclick={() => editor?.chain().focus().toggleHeading({ level: 3 }).run()}><Icon name="heading-3" size={14} /></button>
      <span class="sep"></span>
      <button type="button" class:on={active.bullet} title={__("Bulleted list")} onclick={() => editor?.chain().focus().toggleBulletList().run()}><Icon name="list" size={14} /></button>
      <button type="button" class:on={active.ordered} title={__("Numbered list")} onclick={() => editor?.chain().focus().toggleOrderedList().run()}><Icon name="list-ordered" size={14} /></button>
      <button type="button" class:on={active.quote} title={__("Quote")} onclick={() => editor?.chain().focus().toggleBlockquote().run()}><Icon name="quote" size={14} /></button>
      <button type="button" class:on={active.code} title={__("Code")} onclick={() => editor?.chain().focus().toggleCodeBlock().run()}><Icon name="code" size={14} /></button>
      <span class="sep"></span>
      <button type="button" class:on={active.link} title={__("Link")} onclick={setLink}><Icon name="link" size={14} /></button>
      <label class="upload" title={__("Image")}>
        {#if uploading}<span class="muted small">{__("Uploading…")}</span>{:else}<Icon name="image" size={14} />{/if}
        <input type="file" accept="image/png,image/jpeg,image/gif,image/webp" onchange={pickImage} disabled={uploading} />
      </label>
    </div>
    <div class="surface" bind:this={host} {id}></div>
  </div>
{/if}

<style>
  .richtext {
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-input, #fff);
    overflow: hidden;
  }
  .richtext.error { border-color: var(--danger, #dc2626); }
  .toolbar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 2px;
    padding: 4px;
    border-bottom: 1px solid var(--border);
    background: var(--bg-subtle, #f8fafc);
  }
  .toolbar button,
  .toolbar .upload {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 26px;
    height: 26px;
    padding: 0 5px;
    border: 0;
    border-radius: 4px;
    background: transparent;
    color: var(--text-muted, #64748b);
    cursor: pointer;
  }
  .toolbar button:hover,
  .toolbar .upload:hover { background: var(--bg-hover, #e2e8f0); }
  .toolbar button.on { background: var(--bg-hover, #e2e8f0); color: var(--text, #0f172a); }
  .toolbar .upload input { display: none; }
  .sep { width: 1px; height: 16px; margin: 0 4px; background: var(--border); }
  .surface :global(.tiptap) {
    min-height: 140px;
    padding: 8px 10px;
    outline: none;
    font-size: 13px;
    line-height: 1.55;
  }
  .surface :global(.tiptap > :first-child) { margin-top: 0; }
  .surface :global(.tiptap > :last-child) { margin-bottom: 0; }
  .surface :global(.tiptap p) { margin: 0 0 8px; }
  .surface :global(.tiptap h2) { font-size: 16px; margin: 12px 0 6px; }
  .surface :global(.tiptap h3) { font-size: 14px; margin: 10px 0 6px; }
  .surface :global(.tiptap ul),
  .surface :global(.tiptap ol) { margin: 0 0 8px; padding-left: 20px; }
  .surface :global(.tiptap blockquote) {
    margin: 0 0 8px;
    padding-left: 10px;
    border-left: 3px solid var(--border);
    color: var(--text-muted, #64748b);
  }
  .surface :global(.tiptap pre) {
    padding: 8px 10px;
    border-radius: 4px;
    background: var(--bg-subtle, #f1f5f9);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
    overflow-x: auto;
  }
  .surface :global(.tiptap img) { max-width: 100%; border-radius: 4px; }
  .richtext-read {
    min-height: 0;
    height: auto;
    padding: 8px 10px;
    font-size: 13px;
    line-height: 1.55;
    background: var(--bg-subtle, #f8fafc);
  }
  .richtext-read :global(p) { margin: 0 0 8px; }
  .richtext-read :global(:last-child) { margin-bottom: 0; }
  .richtext-read :global(img) { max-width: 100%; }
</style>
