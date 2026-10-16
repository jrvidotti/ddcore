<script lang="ts">
  // The Feedback modal: a bug, an improvement or a feature request for the
  // site's developers, with screenshots, files and audio notes, an AI that
  // fills the form in from them when the site has one, and the list of what
  // this user sent before with the developers' answers.
  import { untrack } from "svelte";
  import { page } from "$app/state";
  import { focusTrap } from "$lib/focus-trap";
  import { api, type FeedbackItem } from "$lib/api";
  import { __, boot } from "$lib/boot.svelte";
  import { showError, toast } from "$lib/ui.svelte";
  import { feedbackState, closeFeedback } from "$lib/feedback.svelte";
  import {
    FEEDBACK_TYPES, MAX_FEEDBACK_FILES, MAX_TITLE, addFiles, applyAssist, assistPayload, buildContext, canAssist,
    emptyFeedback, feedbackPayload, feedbackRoom, feedbackStatusColor, feedbackTypeDef, isFeedbackType,
    screenshotName, transcriptText, validateFeedback, type FeedbackValues,
  } from "$lib/feedback";
  import { recentClientErrors } from "$lib/client-errors";
  import { formatFileSize } from "$lib/controls/attach-state";
  import { formatDatetime } from "$lib/format";
  import AudioRecorder from "$lib/controls/AudioRecorder.svelte";
  import AudioPreview from "$lib/controls/AudioPreview.svelte";
  import { voiceNoteName } from "$lib/controls/audio-recorder-state";
  import Icon from "./Icon.svelte";

  let tab = $state<"send" | "mine">("send");
  let values = $state<FeedbackValues>(emptyFeedback());
  let errors = $state<Record<string, string>>({});
  let files = $state<File[]>([]);
  let recordings = $state<File[]>([]);
  /** Recording name → its text, as the AI transcribed it; sent back instead of the audio. */
  let transcripts = $state<Record<string, string>>({});
  let assisting = $state(false);
  /** The fields the AI filled in, marked until the person edits them. */
  let aiFilled = $state<string[]>([]);
  /** The form before the AI filled it in, for Undo. */
  let beforeAssist = $state<FeedbackValues | null>(null);
  let recordingSeq = 0;
  let sendUrl = $state(true);
  let sendContext = $state(true);
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
  /** Recordings count toward the limit: each is one more file on the request. */
  const room = $derived(feedbackRoom(files, recordings));
  const locked = $derived(busy || assisting);
  const assistOn = $derived(!!boot.data?.site?.feedbackAssist);

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
    const r = addFiles(files, incoming, MAX_FEEDBACK_FILES - recordings.length);
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
    if (locked) return;
    take(Array.from(e.dataTransfer?.files ?? []));
  }

  function onDragOver(e: DragEvent) {
    if (!e.dataTransfer || !Array.from(e.dataTransfer.types ?? []).includes("Files")) return;
    e.preventDefault();
    dragging = true;
  }

  // a screenshot pasted anywhere in the modal becomes an attachment
  function onPaste(e: ClipboardEvent) {
    if (tab !== "send" || locked) return;
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

  function onRecorded(blob: Blob, mime: string) {
    if (room <= 0) {
      toast(__("Up to {0} files per feedback", [MAX_FEEDBACK_FILES]), { indicator: "orange" });
      return;
    }
    recordingSeq++;
    recordings = [...recordings, new File([blob], voiceNoteName(mime, recordingSeq), { type: mime })];
  }

  function removeRecording(i: number) {
    recordings = recordings.filter((_, j) => j !== i);
  }

  /** An edit by the person takes the AI's mark off the field. */
  function edited(name: string) {
    if (aiFilled.includes(name)) aiFilled = aiFilled.filter((n) => n !== name);
  }

  async function assist() {
    if (locked) return;
    assisting = true;
    try {
      const known: Record<string, string> = {};
      for (const r of recordings) if (r.name in transcripts) known[r.name] = transcripts[r.name];
      const data = assistPayload(values, {
        sendUrl, url: pageUrl, sendContext, context: sendContext ? currentContext() : undefined, transcripts: known,
      });
      const res = await api.assistFeedback(data, [...files, ...recordings.filter((r) => !(r.name in known))]);
      for (const t of res.transcripts ?? []) transcripts[t.name] = t.text;
      const before = $state.snapshot(values) as FeedbackValues;
      const applied = applyAssist(values, res);
      values = applied.values;
      aiFilled = applied.changed;
      beforeAssist = before;
      errors = {};
      toast(applied.typeChanged
        ? __("Review the fields filled in by the AI. It changed the type to {0}.", [__(applied.values.feedback_type)])
        : __("Review the fields filled in by the AI."), { indicator: "green" });
    } catch (e) {
      showError(e);
    } finally {
      assisting = false;
    }
  }

  function undoAssist() {
    if (!beforeAssist) return;
    values = beforeAssist;
    beforeAssist = null;
    aiFilled = [];
  }

  function reset() {
    values = emptyFeedback(values.feedback_type);
    errors = {};
    files = [];
    recordings = [];
    transcripts = {};
    aiFilled = [];
    beforeAssist = null;
    recordingSeq = 0;
    sendContext = true;
    showContext = false;
    screenshots = 0;
  }

  async function submit() {
    if (locked) return;
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
        transcript: transcriptText(recordings, transcripts),
      });
      await api.submitFeedback(data, [...files, ...recordings]);
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
          <p class="intro muted">{__("Tell the developers about a problem or an idea. Be specific: what you did, what you expected and what happened. Screenshots and audio notes help.")}</p>

          <div class="types" role="radiogroup" aria-label={__("Type")}>
            {#each FEEDBACK_TYPES as t (t.value)}
              <button
                type="button"
                role="radio"
                class="type"
                class:on={values.feedback_type === t.value}
                aria-checked={values.feedback_type === t.value}
                disabled={locked}
                onclick={() => setType(t.value)}
              >
                <Icon name={t.icon} size={18} />
                <span>{__(t.label)}</span>
              </button>
            {/each}
          </div>
          <div class="hint small muted">{__(def.hint)}</div>

          <div class="field attach-card">
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
              <button type="button" class="btn sm" onclick={() => fileInput?.click()} disabled={locked || room <= 0}>
                <Icon name="paperclip" size={13} /> {__("Choose files")}
              </button>
              <AudioRecorder onrecorded={onRecorded} disabled={locked || room <= 0} />
              <span class="small muted">{__("or drag them here, or paste a screenshot")}</span>
              <input bind:this={fileInput} type="file" multiple hidden onchange={onPick} />
            </div>
            {#if recordings.length}
              <ul class="files recordings">
                {#each recordings as r, i (r.name)}
                  <li>
                    <Icon name="mic" size={14} />
                    <AudioPreview file={r} />
                    <span class="small muted" title={transcripts[r.name] ?? ""}>{r.name}</span>
                    <button type="button" class="btn icon sm" onclick={() => removeRecording(i)} disabled={locked} aria-label={__("Remove {0}", [r.name])}><Icon name="x" size={12} /></button>
                  </li>
                {/each}
              </ul>
            {/if}
            {#if files.length}
              <ul class="files">
                {#each files as f, i (i + ":" + f.name)}
                  <li>
                    <Icon name={f.type.startsWith("image/") ? "image" : "file"} size={14} />
                    <span class="fname" title={f.name}>{f.name}</span>
                    <span class="small muted">{formatFileSize(f.size)}</span>
                    <button type="button" class="btn icon sm" onclick={() => removeFile(i)} disabled={locked} aria-label={__("Remove {0}", [f.name])}><Icon name="x" size={12} /></button>
                  </li>
                {/each}
              </ul>
            {/if}
            <div class="desc">{__("{0} of {1} files", [files.length + recordings.length, MAX_FEEDBACK_FILES])}</div>
          </div>

          {#if assistOn}
            <div class="assist">
              <div class="assist-actions">
                <button type="button" class="btn sm assist-btn" onclick={assist} disabled={locked || !canAssist(values, files, recordings)}>
                  <Icon name="wand-sparkles" size={14} /> {assisting ? __("Filling in…") : __("Fill in with AI")}
                </button>
                {#if beforeAssist && !assisting}
                  <button type="button" class="link-btn small" onclick={undoAssist}>{__("Undo")}</button>
                {/if}
              </div>
              <div class="small muted">{__("Be detailed: record audio and attach screenshots telling what you did, what you expected and what happened, and try to cover every field below. The AI fills in the form for you to review before sending.")}</div>
              <div class="small muted">{__("Your audio notes and screenshots, and the page address and context data if checked below, are sent to an AI service.")}</div>
            </div>
          {/if}

          <div class="field">
            <label for="feedback-field-title">{__("Title")}<span class="req">*</span></label>
            <input id="feedback-field-title" class="input" class:error={!!errors.title} class:ai-filled={aiFilled.includes("title")} maxlength={MAX_TITLE} bind:value={values.title} oninput={() => edited("title")} disabled={locked} />
            {#if errors.title}<div class="err">{__(errors.title)}</div>{/if}
          </div>
          <div class="field">
            <label for="feedback-field-description">{__("Description")}<span class="req">*</span></label>
            <textarea id="feedback-field-description" class="input" class:error={!!errors.description} class:ai-filled={aiFilled.includes("description")} rows="4" bind:value={values.description} oninput={() => edited("description")} disabled={locked}></textarea>
            {#if errors.description}<div class="err">{__(errors.description)}</div>{/if}
          </div>

          <div class="form-row">
            {#each def.fields as f (f.name)}
              <div class="form-cell" class:w-50={f.half}>
                <div class="field">
                  <label for="feedback-field-{f.name}">{__(f.label)}</label>
                  {#if f.kind === "select"}
                    <select id="feedback-field-{f.name}" class="input" class:ai-filled={aiFilled.includes(f.name)} bind:value={values[f.name]} onchange={() => edited(f.name)} disabled={locked}>
                      <option value=""></option>
                      {#each f.options ?? [] as o}<option value={o}>{__(o)}</option>{/each}
                    </select>
                  {:else if f.kind === "textarea"}
                    <textarea id="feedback-field-{f.name}" class="input" class:ai-filled={aiFilled.includes(f.name)} rows="3" bind:value={values[f.name]} oninput={() => edited(f.name)} disabled={locked}></textarea>
                  {:else}
                    <input id="feedback-field-{f.name}" class="input" class:ai-filled={aiFilled.includes(f.name)} bind:value={values[f.name]} oninput={() => edited(f.name)} disabled={locked} />
                  {/if}
                </div>
              </div>
            {/each}
          </div>

          <div class="field check">
            <div class="check-control">
              <input id="feedback-send-url" type="checkbox" bind:checked={sendUrl} disabled={locked} />
              <label for="feedback-send-url">{__("Send the address of this page")}</label>
            </div>
            {#if pageUrl}<div class="desc url" class:off={!sendUrl}>{pageUrl}</div>{/if}
          </div>
          <div class="field check">
            <div class="check-control">
              <input id="feedback-send-context" type="checkbox" bind:checked={sendContext} disabled={locked} />
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
          <button type="button" class="btn primary" onclick={submit} disabled={locked}>
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
    padding: 10px 8px; border: 1px solid var(--border); border-radius: 8px; background: var(--surface);
    cursor: pointer; font-size: 13px; color: var(--text); text-align: center;
  }
  .type:hover { background: var(--hover); }
  .type.on { border-color: var(--primary); background: var(--active); color: var(--primary); font-weight: 500; }
  .type:disabled { cursor: default; opacity: .6; }
  .hint { margin: 6px 0 14px; }
  .dropzone {
    display: flex; align-items: center; gap: 10px; flex-wrap: wrap; padding: 12px;
    border: 1px dashed var(--border); border-radius: 8px; background: var(--surface-alt);
  }
  .dropzone.dragging { border-color: var(--primary); background: var(--primary-tint); }
  .files { list-style: none; margin: 8px 0 0; padding: 0; display: flex; flex-direction: column; gap: 4px; }
  .files li { display: flex; align-items: center; gap: 8px; font-size: 13px; min-width: 0; }
  .fname { flex: 1; min-width: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .recordings li { flex-wrap: wrap; }
  .assist {
    display: flex; flex-direction: column; gap: 4px; margin: 0 0 16px; padding: 10px 12px;
    border: 1px solid var(--border); border-radius: 8px; background: var(--primary-tint);
  }
  .assist-actions { display: flex; align-items: center; gap: 12px; }
  .assist-actions .link-btn { margin-left: 0; }
  .assist-btn { color: var(--primary); border-color: var(--primary); }
  .input.ai-filled { border-color: var(--primary); background: var(--primary-tint); }
  .field.check { margin-top: 0; }
  .check-control { flex-wrap: wrap; }
  .url { overflow-wrap: anywhere; }
  .url.off { text-decoration: line-through; }
  .link-btn { border: 0; background: none; padding: 0; color: var(--primary); cursor: pointer; margin-left: auto; }
  .context-preview {
    margin: 6px 0 0; padding: 8px; max-height: 240px; overflow: auto; font-size: 11px; line-height: 1.45;
    background: var(--surface-alt); border: 1px solid var(--border); border-radius: 6px; white-space: pre-wrap; overflow-wrap: anywhere;
  }
  .mine { list-style: none; margin: 0; padding: 0; display: flex; flex-direction: column; gap: 10px; }
  .mine li { border: 1px solid var(--border); border-radius: 8px; padding: 10px 12px; display: flex; flex-direction: column; gap: 4px; }
  .mine-head { display: flex; align-items: center; gap: 8px; min-width: 0; }
  .mine-title { flex: 1; min-width: 0; font-weight: 500; font-size: 13px; overflow-wrap: anywhere; }
  .response { margin-top: 4px; padding: 8px 10px; background: var(--surface-alt); border-radius: 6px; font-size: 13px; white-space: pre-wrap; overflow-wrap: anywhere; }
  .err { color: var(--red); }

  @media (max-width: 600px) {
    .feedback-bg { padding: 8px; }
    .modal .body, .modal .head, .modal .foot { padding-left: 14px; padding-right: 14px; }
    .tabs { padding: 4px 14px 0; }
    .type { flex-direction: column; gap: 4px; padding: 8px 4px; font-size: 12px; }
    .link-btn { margin-left: 0; }
  }
</style>
