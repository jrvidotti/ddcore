<script lang="ts">
  // The Feedback modal: a bug, an improvement or a feature request for the
  // site's developers, with screenshots, files and an audio note, and the
  // list of what this user sent before with the developers' answers.
  import { untrack } from "svelte";
  import { page } from "$app/state";
  import { focusTrap } from "$lib/focus-trap";
  import { api, type FeedbackItem } from "$lib/api";
  import { __, boot } from "$lib/boot.svelte";
  import { showError, toast } from "$lib/ui.svelte";
  import { feedbackState, closeFeedback } from "$lib/feedback.svelte";
  import {
    FEEDBACK_TYPES, MAX_FEEDBACK_FILES, MAX_TITLE, addFiles, buildContext, emptyFeedback,
    feedbackPayload, feedbackStatusColor, feedbackTypeDef, isFeedbackType, screenshotName,
    validateFeedback, type FeedbackValues,
  } from "$lib/feedback";
  import { recentClientErrors } from "$lib/client-errors";
  import { formatFileSize } from "$lib/controls/attach-state";
  import { formatDatetime } from "$lib/format";
  import AudioRecorder from "$lib/controls/AudioRecorder.svelte";
  import Icon from "./Icon.svelte";

  let tab = $state<"send" | "mine">("send");
  let values = $state<FeedbackValues>(emptyFeedback());
  let errors = $state<Record<string, string>>({});
  let files = $state<File[]>([]);
  let audio = $state<File | null>(null);
  let sendUrl = $state(true);
  let sendContext = $state(false);
  let showContext = $state(false);
  let busy = $state(false);
  let dragging = $state(false);
  let pageUrl = $state("");
  let screenshots = 0;
  let fileInput: HTMLInputElement | null = $state(null);

  let mine = $state<FeedbackItem[] | null>(null);
  let mineLoading = $state(false);
  let mineError = $state("");

  const def = $derived(feedbackTypeDef(values.feedback_type));
  /** The note counts toward the limit: it is one more file on the request. */
  const fileRoom = $derived(MAX_FEEDBACK_FILES - (audio ? 1 : 0));

  // each opening applies its preset (ddcore.ui.openFeedback); a draft left
  // by closing the modal is kept
  let appliedSeq = 0;
  $effect(() => {
    if (!feedbackState.open) return;
    const seq = feedbackState.seq;
    if (seq === appliedSeq) return;
    appliedSeq = seq;
    untrack(() => {
      const p = feedbackState.preset;
      tab = "send";
      if (p?.type && isFeedbackType(p.type)) values.feedback_type = p.type;
      if (p?.title) values.title = String(p.title).slice(0, MAX_TITLE);
      pageUrl = page.url?.href ?? "";
    });
  });

  function currentContext() {
    return buildContext({
      boot: boot.data,
      path: page.url?.pathname,
      params: page.params as Record<string, string | undefined>,
      viewport: typeof window !== "undefined" ? { width: window.innerWidth, height: window.innerHeight } : undefined,
      userAgent: typeof navigator !== "undefined" ? navigator.userAgent : undefined,
      timezone: (() => { try { return Intl.DateTimeFormat().resolvedOptions().timeZone; } catch { return undefined; } })(),
      errors: recentClientErrors(),
    });
  }
  const contextPreview = $derived(showContext ? JSON.stringify(currentContext(), null, 2) : "");

  function close() {
    if (busy) return;
    closeFeedback();
  }

  function setType(t: FeedbackValues["feedback_type"]) {
    values.feedback_type = t;
  }

  function take(incoming: File[]) {
    if (!incoming.length) return;
    const r = addFiles(files, incoming, fileRoom);
    files = r.files;
    if (r.dropped) toast(__("Up to {0} files per feedback", [MAX_FEEDBACK_FILES]), { indicator: "orange" });
  }

  function onPick(e: Event) {
    const input = e.target as HTMLInputElement;
    take(Array.from(input.files ?? []));
    input.value = "";
  }

  function onDrop(e: DragEvent) {
    e.preventDefault();
    dragging = false;
    if (busy) return;
    take(Array.from(e.dataTransfer?.files ?? []));
  }

  function onDragOver(e: DragEvent) {
    if (!e.dataTransfer || !Array.from(e.dataTransfer.types ?? []).includes("Files")) return;
    e.preventDefault();
    dragging = true;
  }

  // a screenshot pasted anywhere in the modal becomes an attachment
  function onPaste(e: ClipboardEvent) {
    if (tab !== "send" || busy) return;
    const items = Array.from(e.clipboardData?.items ?? []);
    const pasted: File[] = [];
    for (const it of items) {
      if (it.kind !== "file") continue;
      const f = it.getAsFile();
      if (!f) continue;
      if (it.type.startsWith("image/")) {
        screenshots++;
        pasted.push(new File([f], screenshotName(screenshots, it.type), { type: it.type }));
      } else pasted.push(f);
    }
    if (!pasted.length) return;
    e.preventDefault();
    take(pasted);
  }

  function removeFile(i: number) {
    files = files.filter((_, j) => j !== i);
  }

  function reset() {
    values = emptyFeedback(values.feedback_type);
    errors = {};
    files = [];
    audio = null;
    sendContext = false;
    showContext = false;
    screenshots = 0;
  }

  async function submit() {
    if (busy) return;
    errors = validateFeedback(values);
    if (Object.keys(errors).length) {
      const first = Object.keys(errors)[0];
      document.getElementById(`feedback-field-${first}`)?.focus();
      return;
    }
    busy = true;
    try {
      const data = feedbackPayload(values, {
        sendUrl, url: pageUrl, sendContext, context: sendContext ? currentContext() : undefined,
      });
      await api.submitFeedback(data, audio ? [...files, audio] : [...files]);
      toast(__("Thanks! Your feedback was sent."), { indicator: "green" });
      reset();
      tab = "mine";
      void loadMine();
    } catch (e) {
      showError(e);
    } finally {
      busy = false;
    }
  }

  async function loadMine() {
    mineLoading = true;
    mineError = "";
    try {
      mine = (await api.myFeedback()) ?? [];
    } catch (e: any) {
      mineError = String(e?.message || e);
    } finally {
      mineLoading = false;
    }
  }

  function showTab(t: "send" | "mine") {
    tab = t;
    if (t === "mine" && !mineLoading) void loadMine();
  }

  function onKeydown(e: KeyboardEvent) {
    if (e.key !== "Escape") return;
    e.stopPropagation();
    close();
  }

  const typeIcon = (t: string) => feedbackTypeDef(t).icon;
</script>

{#if feedbackState.open}
  <div
    class="modal-bg feedback-bg"
    use:focusTrap
    role="dialog"
    aria-modal="true"
    aria-labelledby="feedback-title"
    tabindex="-1"
    onclick={(e) => e.target === e.currentTarget && close()}
    onkeydown={onKeydown}
    onpaste={onPaste}
  >
    <div class="modal md feedback">
      <div class="head">
        <h3 id="feedback-title" class="title">
          <Icon name="message-square" size={18} />
          {__("Feedback")}
        </h3>
        <button type="button" class="btn icon" onclick={close} aria-label={__("Close")}><Icon name="x" /></button>
      </div>

      <div class="tabs" role="tablist">
        <button type="button" role="tab" aria-selected={tab === "send"} class:on={tab === "send"} onclick={() => showTab("send")}>{__("Send")}</button>
        <button type="button" role="tab" aria-selected={tab === "mine"} class:on={tab === "mine"} onclick={() => showTab("mine")}>{__("My feedback")}</button>
      </div>

      {#if tab === "send"}
        <div class="body">
          <p class="intro muted">{__("Tell the developers about a problem or an idea. Be specific: what you did, what you expected and what happened. Screenshots and an audio note help.")}</p>

          <div class="types" role="radiogroup" aria-label={__("Type")}>
            {#each FEEDBACK_TYPES as t (t.value)}
              <button
                type="button"
                role="radio"
                class="type"
                class:on={values.feedback_type === t.value}
                aria-checked={values.feedback_type === t.value}
                disabled={busy}
                onclick={() => setType(t.value)}
              >
                <Icon name={t.icon} size={18} />
                <span>{__(t.label)}</span>
              </button>
            {/each}
          </div>
          <div class="hint small muted">{__(def.hint)}</div>

          <div class="field">
            <label for="feedback-field-title">{__("Title")}<span class="req">*</span></label>
            <input id="feedback-field-title" class="input" class:error={!!errors.title} maxlength={MAX_TITLE} bind:value={values.title} disabled={busy} />
            {#if errors.title}<div class="err">{__(errors.title)}</div>{/if}
          </div>
          <div class="field">
            <label for="feedback-field-description">{__("Description")}<span class="req">*</span></label>
            <textarea id="feedback-field-description" class="input" class:error={!!errors.description} rows="4" bind:value={values.description} disabled={busy}></textarea>
            {#if errors.description}<div class="err">{__(errors.description)}</div>{/if}
          </div>

          <div class="form-row">
            {#each def.fields as f (f.name)}
              <div class="form-cell" class:w-50={f.half}>
                <div class="field">
                  <label for="feedback-field-{f.name}">{__(f.label)}</label>
                  {#if f.kind === "select"}
                    <select id="feedback-field-{f.name}" class="input" bind:value={values[f.name]} disabled={busy}>
                      <option value=""></option>
                      {#each f.options ?? [] as o}<option value={o}>{__(o)}</option>{/each}
                    </select>
                  {:else if f.kind === "textarea"}
                    <textarea id="feedback-field-{f.name}" class="input" rows="3" bind:value={values[f.name]} disabled={busy}></textarea>
                  {:else}
                    <input id="feedback-field-{f.name}" class="input" bind:value={values[f.name]} disabled={busy} />
                  {/if}
                </div>
              </div>
            {/each}
          </div>

          <div class="field">
            <span class="label">{__("Attachments")}</span>
            <div
              class="dropzone"
              class:dragging
              role="group"
              aria-label={__("Attachments")}
              ondragover={onDragOver}
              ondragleave={() => (dragging = false)}
              ondrop={onDrop}
            >
              <button type="button" class="btn sm" onclick={() => fileInput?.click()} disabled={busy || files.length >= fileRoom}>
                <Icon name="paperclip" size={13} /> {__("Choose files")}
              </button>
              <span class="small muted">{__("or drag them here, or paste a screenshot")}</span>
              <input bind:this={fileInput} type="file" multiple hidden onchange={onPick} />
            </div>
            {#if files.length}
              <ul class="files">
                {#each files as f, i (i + ":" + f.name)}
                  <li>
                    <Icon name={f.type.startsWith("image/") ? "image" : "file"} size={14} />
                    <span class="fname" title={f.name}>{f.name}</span>
                    <span class="small muted">{formatFileSize(f.size)}</span>
                    <button type="button" class="btn icon sm" onclick={() => removeFile(i)} disabled={busy} aria-label={__("Remove {0}", [f.name])}><Icon name="x" size={12} /></button>
                  </li>
                {/each}
              </ul>
            {/if}
            <div class="desc">{__("{0} of {1} files", [files.length + (audio ? 1 : 0), MAX_FEEDBACK_FILES])}</div>
          </div>

          <div class="field audio">
            <AudioRecorder value={audio} onchange={(f) => (audio = f)} disabled={busy || files.length >= MAX_FEEDBACK_FILES} />
          </div>

          <div class="field check">
            <div class="check-control">
              <input id="feedback-send-url" type="checkbox" bind:checked={sendUrl} disabled={busy} />
              <label for="feedback-send-url">{__("Send the address of this page")}</label>
            </div>
            {#if pageUrl}<div class="desc url" class:off={!sendUrl}>{pageUrl}</div>{/if}
          </div>
          <div class="field check">
            <div class="check-control">
              <input id="feedback-send-context" type="checkbox" bind:checked={sendContext} disabled={busy} />
              <label for="feedback-send-context">{__("Send context data")}</label>
              <button type="button" class="link-btn small" aria-expanded={showContext} onclick={() => (showContext = !showContext)}>
                {showContext ? __("Hide") : __("See what will be sent")}
              </button>
            </div>
            <div class="desc">{__("Your user, roles and language, the site's version and apps, your browser and the last errors on this page.")}</div>
            {#if showContext}<pre class="context-preview">{contextPreview}</pre>{/if}
          </div>
        </div>
        <div class="foot">
          <button type="button" class="btn" onclick={close} disabled={busy}>{__("Cancel")}</button>
          <button type="button" class="btn primary" onclick={submit} disabled={busy}>
            <Icon name="send" size={14} /> {busy ? __("Sending…") : __("Send")}
          </button>
        </div>
      {:else}
        <div class="body">
          {#if mineLoading && !mine}
            <div class="small muted">{__("Loading...")}</div>
          {:else if mineError}
            <div class="err small" role="alert">{mineError}</div>
          {:else if !mine?.length}
            <div class="small muted">{__("You have not sent any feedback yet.")}</div>
          {:else}
            <ul class="mine">
              {#each mine as item (item.id)}
                <li>
                  <div class="mine-head">
                    <Icon name={typeIcon(item.feedback_type)} size={14} />
                    <span class="mine-title">{item.title}</span>
                    <span class="indicator {feedbackStatusColor(item.status)}">{__(item.status)}</span>
                  </div>
                  <div class="small muted">{__(item.feedback_type)} · {formatDatetime(item.creation)}</div>
                  {#if item.response}
                    <div class="response"><span class="small muted">{__("Response")}</span><div>{item.response}</div></div>
                  {/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
        <div class="foot">
          <button type="button" class="btn" onclick={close}>{__("Close")}</button>
        </div>
      {/if}
    </div>
  </div>
{/if}

<style>
  .title { display: flex; align-items: center; gap: 8px; }
  .tabs { display: flex; gap: 2px; padding: 6px 20px 0; border-bottom: 1px solid var(--border); }
  .tabs button {
    border: 0; background: none; padding: 8px 12px; font-size: 13px; cursor: pointer;
    color: var(--muted); border-bottom: 2px solid transparent; margin-bottom: -1px;
  }
  .tabs button.on { color: var(--text); border-bottom-color: var(--primary); font-weight: 500; }
  .intro { margin: 0 0 12px; font-size: 13px; line-height: 1.5; }
  .types { display: flex; gap: 8px; }
  .type {
    flex: 1 1 0; min-width: 0; display: flex; align-items: center; justify-content: center; gap: 8px;
    padding: 10px 8px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface, #fff);
    cursor: pointer; font-size: 13px; color: var(--text); text-align: center;
  }
  .type:hover { background: #f3f4f6; }
  .type.on { border-color: var(--primary); background: rgba(37, 99, 235, .08); color: var(--primary); font-weight: 500; }
  .type:disabled { cursor: default; opacity: .6; }
  .hint { margin: 6px 0 14px; }
  .dropzone {
    display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 12px;
    border: 1px dashed var(--border); border-radius: 8px; background: #fafafa;
  }
  .dropzone.dragging { border-color: var(--primary); background: rgba(37, 99, 235, .06); }
  .files { list-style: none; margin: 8px 0 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .files li { display: flex; align-items: center; gap: 8px; font-size: 13px; min-width: 0; }
  .fname { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .audio { margin-bottom: 16px; }
  .field.check { margin-top: 0; }
  .check-control { flex-wrap: wrap; }
  .url { overflow-wrap: anywhere; }
  .url.off { text-decoration: line-through; }
  .link-btn { border: 0; background: none; padding: 0; color: var(--primary); cursor: pointer; margin-left: auto; }
  .context-preview {
    margin: 6px 0 0; padding: 8px; max-height: 240px; overflow: auto; font-size: 11px; line-height: 1.45;
    background: #f8fafc; border: 1px solid var(--border); border-radius: 6px; white-space: pre-wrap; overflow-wrap: anywhere;
  }
  .mine { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .mine li { border: 1px solid var(--border); border-radius: 8px; padding: 10px 12px; display: flex; flex-direction: column; gap: 4px; }
  .mine-head { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .mine-title { flex: 1; min-width: 0; font-weight: 500; font-size: 13px; overflow-wrap: anywhere; }
  .response { margin-top: 4px; padding: 8px 10px; background: #f8fafc; border-radius: 6px; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
  .err { color: var(--red); }

  @media (max-width: 600px) {
    .feedback-bg { padding: 8px; }
    .modal .body, .modal .head, .modal .foot { padding-left: 14px; padding-right: 14px; }
    .tabs { padding: 4px 14px 0; }
    .type { flex-direction: column; gap: 4px; padding: 8px 4px; font-size: 12px; }
    .link-btn { margin-left: 0; }
  }
</style>
