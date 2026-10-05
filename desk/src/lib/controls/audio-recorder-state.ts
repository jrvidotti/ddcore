/**
 * The audio note on a feedback: what the browser can record, in which
 * format, and how long. The recorder needs the microphone (getUserMedia,
 * which only a secure context has) and MediaRecorder; without them the
 * recorder is not shown at all.
 */

/** The formats tried, best first: Chromium and Firefox take WebM/Opus, Safari MP4. */
export const AUDIO_MIME_CANDIDATES = ["audio/webm;codecs=opus", "audio/webm", "audio/mp4", "audio/ogg"];

/** A note stops by itself after this long. */
export const MAX_RECORDING_SECONDS = 5 * 60;

export function recordingAvailable(win: any = typeof window === "undefined" ? undefined : window): boolean {
  return !!win && !!win.isSecureContext && !!win.navigator?.mediaDevices?.getUserMedia && typeof win.MediaRecorder === "function";
}

/**
 * The first candidate the browser records; "" lets MediaRecorder choose, for
 * a browser that has it but cannot answer isTypeSupported.
 */
export function pickAudioMime(rec: { isTypeSupported?: (t: string) => boolean } | undefined): string {
  if (!rec || typeof rec.isTypeSupported !== "function") return "";
  return AUDIO_MIME_CANDIDATES.find((t) => { try { return rec.isTypeSupported!(t); } catch { return false; } }) ?? "";
}

/** The file extension a recording of `mime` gets. */
export function audioExtension(mime: string): string {
  const base = String(mime || "").split(";")[0].trim().toLowerCase();
  if (base === "audio/mp4" || base === "audio/aac" || base === "audio/x-m4a") return "m4a";
  if (base === "audio/ogg") return "ogg";
  if (base === "audio/mpeg") return "mp3";
  return "webm";
}

export function voiceNoteName(mime: string): string {
  return `voice-note.${audioExtension(mime)}`;
}

/** Seconds as mm:ss. */
export function formatElapsed(seconds: number): string {
  const s = Math.max(0, Math.floor(seconds || 0));
  const m = Math.floor(s / 60);
  return `${m < 10 ? "0" + m : m}:${String(s % 60).padStart(2, "0")}`;
}

/** Whether getUserMedia failed because the person (or the browser) refused the microphone. */
export function isPermissionDenied(err: any): boolean {
  const name = String(err?.name || "");
  return name === "NotAllowedError" || name === "PermissionDeniedError" || name === "SecurityError";
}

/** Stops every track of a stream, which turns the browser's microphone indicator off. */
export function stopStream(stream: { getTracks(): { stop(): void }[] } | null | undefined) {
  if (!stream) return;
  for (const t of stream.getTracks()) { try { t.stop(); } catch { /* already stopped */ } }
}
